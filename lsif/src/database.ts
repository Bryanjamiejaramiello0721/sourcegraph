import * as lsp from 'vscode-languageserver-protocol'
import { DocumentModel, DefModel, MetaModel, RefModel, PackageModel } from './models'
import { Connection } from 'typeorm'
import { groupBy } from 'lodash'
import { decodeJSON } from './encoding'
import { MonikerData, RangeData, ResultSetData, DocumentData, FlattenedRange } from './entities'
import { Id } from 'lsif-protocol'
import { makeFilename } from './backend'
import { XrepoDatabase } from './xrepo'
import { ConnectionCache, DocumentCache } from './cache'

/**
 * A wrapper around operations for single repository/commit pair.
 */
export class Database {
    /**
     * Create a new `Database` with the given cross-repo database instance and the
     * filename of the database that contains data for a particular repository/commit.
     *
     * @param xrepoDatabase The cross-repo database.
     * @param connectionCache The cache of SQLite connections.
     * @param documentCache The cache of loaded document.
     * @param databasePath The path to the database file.
     */
    constructor(
        private xrepoDatabase: XrepoDatabase,
        private connectionCache: ConnectionCache,
        private documentCache: DocumentCache,
        private databasePath: string
    ) {}

    /**
     * Determine if data exists for a particular document in this database.
     *
     * @param path The path of the document.
     */
    public async exists(path: string): Promise<boolean> {
        return (await this.findDocument(path)) !== undefined
    }

    /**
     * Return the location for the definition of the reference at the given position.
     *
     * @param path The path of the document to which the position belongs.
     * @param position The current hover position.
     */
    public async definitions(path: string, position: lsp.Position): Promise<lsp.Location[]> {
        const { document, range } = await this.findRange(path, position)
        if (!document || !range) {
            return []
        }

        // First, we try to find the definition result attached to the range or one
        // of the result sets to which the range is attached.

        const resultData = findResult(document.resultSets, document.definitionResults, range, 'definitionResult')
        if (resultData) {
            // We have a definition result in this database.
            return await this.getLocations(path, document, resultData)
        }

        // Otherwise, we fall back to a moniker search. We get all the monikers attached
        // to the range or a result set to which the range is attached. We process each
        // moniker sequentially in order of priority, where import monikers, if any exist,
        // will be processed first.

        for (const moniker of findMonikers(document.resultSets, document.monikers, range)) {
            if (moniker.kind === 'import') {
                // This symbol was imported from another database. See if we have xrepo
                // definition for it.

                const defs = await this.remoteDefinitions(document, moniker)
                if (defs) {
                    return defs
                }

                continue
            }

            // This symbol was not imported from another database. We search the Defs table
            // of our own database in case there was a definition that wasn't properly
            // attached to a result set but did have the correct monikers attached.

            const defs = await Database.monikerResults(this, DefModel, moniker, path => path)
            if (defs) {
                return defs
            }
        }

        return []
    }

    /**
     * Return a list of locations which reference the definition at the given position.
     *
     * @param path The path of the document to which the position belongs.
     * @param position The current hover position.
     * @param page The page index being requested.
     */
    public async references(
        path: string,
        position: lsp.Position,
        page: number | undefined
    ): Promise<{ data: lsp.Location[]; nextPage: number | null }> {
        const { document, range } = await this.findRange(path, position)
        if (!document || !range) {
            return { data: [], nextPage: null }
        }

        let result: lsp.Location[] = []

        // First, we try to find the reference result attached to the range or one
        // of the result sets to which the range is attached.

        const resultData = findResult(document.resultSets, document.referenceResults, range, 'referenceResult')
        if (resultData) {
            // We have references in this database.
            result = result.concat(await this.getLocations(path, document, resultData))
        }

        // Next, we do a moniker search in two stages, described below. We process each
        // moniker sequentially in order of priority for each stage, where import monikers,
        // if any exist, will be processed first.

        const monikers = findMonikers(document.resultSets, document.monikers, range)

        // First, we search the Refs table of our own database - this search is necessary,
        // but may be unintuitive, but remember that a 'Find References' operation on a
        // reference should also return references to the definition - these are not
        // necessarily fully linked in the LSIF data.

        for (const moniker of monikers) {
            result = result.concat(await Database.monikerResults(this, RefModel, moniker, path => path))
        }

        // Second, we perform an xrepo search for uses of each nonlocal moniker. We stop
        // processing after the first moniker for which we received results. As we process
        // monikers in an order that considers moniker schemes, the first one to get results
        // should be the most desirable.

        for (const moniker of monikers) {
            if (moniker.kind === 'local') {
                continue
            }

            const { remoteResults, nextPage } = await this.remoteReferences(document, moniker, page)
            if (remoteResults) {
                return { data: result.concat(remoteResults), nextPage }
            }
        }

        return { data: locations, nextPage: null }
    }

    /**
     * Return the hover content for the definition or reference at the given position.
     *
     * @param path The path of the document to which the position belongs.
     * @param position The current hover position.
     */
    public async hover(path: string, position: lsp.Position): Promise<lsp.Hover | null> {
        const { document, range } = await this.findRange(path, position)
        if (!document || !range) {
            return null
        }

        // Try to find the hover content attached to the range or one of the result sets to
        // which the range is attached. There is no fall-back search via monikers for this
        // operation.

        const contents = findResult(document.resultSets, document.hovers, range, 'hoverResult')
        if (!contents) {
            return null
        }

        return { contents }
    }

    //
    // Helper Functions

    /**
     * Query the defs or refs table of `db` for items that match the given moniker. Convert
     * each result into an LSP location. The `pathTransformer` function is invoked on each
     * result item to modify the resulting locations.
     *
     * @param db The target database.
     * @param model The constructor for the model type.
     * @param moniker The target moniker.
     * @param pathTransformer The function used to alter location paths.
     */
    private static async monikerResults(
        db: Database,
        model: typeof DefModel | typeof RefModel,
        moniker: MonikerData,
        pathTransformer: (path: string) => string
    ): Promise<lsp.Location[]> {
        const results = await db.withConnection(connection =>
            connection.getRepository<DefModel | RefModel>(model).find({
                where: {
                    scheme: moniker.scheme,
                    identifier: moniker.identifier,
                },
            })
        )

        return results.map(result => lsp.Location.create(pathTransformer(result.documentPath), makeRange(result)))
    }

    /**
     * Convert a set of range results (from a definition or reference query) into a set
     * of LSP ranges. Each range result holds the range Id as well as the document path.
     * For document paths matching the loaded document, find the range data locally. For
     * all other paths, find the document in this database and find the range in that
     * document.
     *
     * @param path The path of the document for this query.
     * @param document The document object for this query.
     * @param resultData A lsit of range ids and the document they belong to.
     */
    private async getLocations(
        path: string,
        document: DocumentData,
        resultData: { documentPath: string; id: Id }[]
    ): Promise<lsp.Location[]> {
        // Group by document path so we only have to load each document once
        const groups = groupBy(resultData, v => v.documentPath)

        let results: lsp.Location[] = []
        for (const documentPath of Object.keys(groups)) {
            // Get all ids for the document path
            const ids = groups[documentPath].map(v => v.id)

            if (documentPath === path) {
                // If the document path is this document, convert the locations directly
                results = results.concat(asLocations(document.ranges, document.orderedRanges, path, ids))
                continue
            }

            // Otherwise, we need to get the correct document
            const sibling = await this.findDocument(documentPath)
            if (!sibling) {
                continue
            }

            // Then finally convert the locations in the sibling document
            results = results.concat(asLocations(sibling.ranges, sibling.orderedRanges, documentPath, ids))
        }

        return results
    }

    /**
     * Find the definition of the target moniker outside of the current database. If the
     * moniker has attached package information, then the xrepo database is queried for
     * the target package. That database is opened, and its def table is queried for the
     * target moniker.
     *
     * @param document The document containing the reference.
     * @param moniker The target moniker.
     */
    private async remoteDefinitions(document: DocumentData, moniker: MonikerData): Promise<lsp.Location[]> {
        if (!moniker.packageInformation) {
            return []
        }

        const packageInformation = document.packageInformation.get(moniker.packageInformation)
        if (!packageInformation) {
            return []
        }

        const packageEntity = await this.xrepoDatabase.getPackage(
            moniker.scheme,
            packageInformation.name,
            packageInformation.version
        )

        if (!packageEntity) {
            return []
        }

        const db = new Database(
            this.xrepoDatabase,
            this.connectionCache,
            this.documentCache,
            makeFilename(packageEntity.repository, packageEntity.commit)
        )

        const pathTransformer = (path: string): string => makeRemoteUri(packageEntity, path)
        return await Database.monikerResults(db, DefModel, moniker, pathTransformer)
    }

    /**
     * Find the references of the target moniker outside of the current database. If the moniker
     * has attached package information, then the xrepo database is queried for the packages that
     * require this particular moniker identifier. These databases are opened, and their ref tables
     * are queried for the target moniker.
     *
     * @param document The document containing the definition.
     * @param moniker The target moniker.
     * @param page The page index being requested.
     */
    private async remoteReferences(
        document: DocumentData,
        moniker: MonikerData,
        page: number | undefined
    ): Promise<{ remoteLocations: lsp.Location[]; nextPage: number | null }> {
        if (!moniker.packageInformation) {
            return { remoteLocations: [], nextPage: null }
        }

        const packageInformation = document.packageInformation.get(moniker.packageInformation)
        if (!packageInformation) {
            return { remoteLocations: [], nextPage: null }
        }

        const { references, nextPage } = await this.xrepoDatabase.getReferences(
            moniker.scheme,
            packageInformation.name,
            packageInformation.version,
            moniker.identifier,
            page
        )

        const promises: Promise<lsp.Location[]>[] = []
        for (const reference of references) {
            const db = new Database(
                this.xrepoDatabase,
                this.connectionCache,
                this.documentCache,
                makeFilename(reference.repository, reference.commit)
            )

            const pathTransformer = (path: string): string => makeRemoteUri(reference, path)
            promises.push(Database.monikerResults(db, RefModel, moniker, pathTransformer))
        }

        const resolved = await Promise.all(promises)
        const remoteLocations = ([] as lsp.Location[]).concat(...resolved)

        return { remoteLocations, nextPage }
    }

    /**
     * Return a parsed document that describes the given path. The result of this
     * method is cached across all database instances.
     *
     * @param path The path of the document.
     */
    private async findDocument(path: string): Promise<DocumentData | undefined> {
        const factory = async (): Promise<DocumentData> => {
            const document = await this.withConnection(connection =>
                connection.getRepository(DocumentModel).findOneOrFail(path)
            )

            return await decodeJSON<DocumentData>(document.value)
        }

        return await this.documentCache.withDocument(`${this.databasePath}::${path}`, factory, document =>
            Promise.resolve(document)
        )
    }

    /**
     * Return a parsed document that describes the given path as well as the range
     * from that document that contains the given position. Returns undefined for
     * both values if one cannot be loaded.
     *
     * @param path The path of the document.
     * @param position The user's hover position.
     */
    private async findRange(
        path: string,
        position: lsp.Position
    ): Promise<{ document: DocumentData | undefined; range: RangeData | undefined }> {
        const document = await this.findDocument(path)
        if (!document) {
            return { document: undefined, range: undefined }
        }

        const range = findRange(document.orderedRanges, position)
        if (!range) {
            return { document: undefined, range: undefined }
        }

        return { document, range }
    }

    /**
     * Invoke `callback` with a SQLite connection object obtained from the
     * cache or created on cache miss.
     *
     * @param callback The function invoke with the SQLite connection.
     */
    private async withConnection<T>(callback: (connection: Connection) => Promise<T>): Promise<T> {
        return await this.connectionCache.withConnection(
            this.databasePath,
            [DefModel, DocumentModel, MetaModel, RefModel],
            callback
        )
    }
}

/**
 * Perform binary search over the ordered ranges of a document, returning
 * the range that includes it (if it exists). LSIF requires that no ranges
 * overlap in a single document. Then, we can compare a position against a
 * range by saying that it's contained within it (what we want), occurs
 * before it, or occurs after it. These later two results let us cut our
 * search space by half each time.
 *
 * @param orderedRanges The ranges of the document, ordered by startLine/startCharacter.
 * @param position The user's hover position.
 */
export function findRange(orderedRanges: RangeData[], position: lsp.Position): RangeData | undefined {
    let lo = 0
    let hi = orderedRanges.length - 1

    while (lo <= hi) {
        const mid = Math.floor((lo + hi) / 2)
        const range = orderedRanges[mid]

        const cmp = comparePosition(range, position)
        if (cmp === 0) {
            return range
        }

        if (cmp < 0) {
            lo = mid + 1
        } else {
            hi = mid - 1
        }
    }

    return undefined
}

/**
 * Return the closest defined `property` related to the given range
 * or result set. This method will walk the `next` chains of the item
 * to find the property on an attached result set if it's not set
 * on the range itself. Note that the `property` on the range and
 * result set objects are simply identifiers, so the real value must
 * be looked up in a secondary data structure `map`.
 *
 * @param resultSets The map of results sets of the document.
 * @param map The map from which to return the property value.
 * @param data The range or result set object.
 * @param property The target property.
 */
export function findResult<T>(
    resultSets: Map<Id, ResultSetData>,
    map: Map<Id, T>,
    data: RangeData | ResultSetData,
    property: 'definitionResult' | 'referenceResult' | 'hoverResult'
): T | undefined {
    for (const current of walkChain(resultSets, data)) {
        const value = current[property]
        if (value) {
            return map.get(value)
        }
    }

    return undefined
}

/**
 * Retrieve all monikers attached to a range or result set.
 *
 * @param resultSets The map of results sets of the document.
 * @param monikers The map of monikers of the document.
 * @param data The range or result set object.
 */
export function findMonikers(
    resultSets: Map<Id, ResultSetData>,
    monikers: Map<Id, MonikerData>,
    data: RangeData | ResultSetData
): MonikerData[] {
    const monikerSet: MonikerData[] = []
    for (const current of walkChain(resultSets, data)) {
        for (const id of current.monikers) {
            const moniker = monikers.get(id)
            if (moniker) {
                monikerSet.push(moniker)
            }
        }
    }

    return sortMonikers(monikerSet)
}

/**
 * Return an iterable of the range and result set items that are attached
 * to the given initial data. The initial data is yielded immediately.
 *
 * @param resultSets The map of results sets of the document.
 * @param data The range or result set object.
 */
export function* walkChain<T>(
    resultSets: Map<Id, ResultSetData>,
    data: RangeData | ResultSetData
): Iterable<RangeData | ResultSetData> {
    let current: RangeData | ResultSetData | undefined = data

    while (current) {
        yield current
        if (!current.next) {
            return
        }

        current = resultSets.get(current.next)
    }
}

/**
 * Sort the monikers by kind, then scheme in order of the following
 * preferences.
 *
 *   - kind: import, local, export
 *   - scheme: npm, tsc
 *
 * @param monikers The list of monikers.
 */
export function sortMonikers(monikers: MonikerData[]): MonikerData[] {
    const monikerKindPreferences = ['import', 'local', 'export']
    const monikerSchemePreferences = ['npm', 'tsc']

    monikers.sort((a, b) => {
        const ord = monikerKindPreferences.indexOf(a.kind) - monikerKindPreferences.indexOf(b.kind)
        if (ord !== 0) {
            return ord
        }

        return monikerSchemePreferences.indexOf(a.scheme) - monikerSchemePreferences.indexOf(b.scheme)
    })

    return monikers
}

/**
 * Convert the given range identifiers into LSP location objects.
 *
 * @param ranges The map of ranges of the document (from identifier to the range's index in `orderedRanges`).
 * @param orderedRanges The ordered ranges of the document.
 * @param uri The location URI.
 * @param ids The set of range identifiers for each resulting location.
 */
export function asLocations(
    ranges: Map<Id, number>,
    orderedRanges: RangeData[],
    uri: string,
    ids: Id[]
): lsp.Location[] {
    const locations = []
    for (const id of ids) {
        const rangeIndex = ranges.get(id)
        if (rangeIndex === undefined) {
            continue
        }

        const range = orderedRanges[rangeIndex]
        locations.push(
            lsp.Location.create(uri, {
                start: { line: range.startLine, character: range.startCharacter },
                end: { line: range.endLine, character: range.endCharacter },
            })
        )
    }

    return locations
}

/**
 * Construct a URI that can be used by the frontend to switch to another
 * directory.
 *
 * @param pkg The target package.
 * @param path The path relative to the project root.
 */
export function makeRemoteUri(pkg: PackageModel, path: string): string {
    const url = new URL(`git://${pkg.repository}`)
    url.search = pkg.commit
    url.hash = path
    return url.href
}

/**
 * Construct an LSP range from a flat range.
 *
 * @param result The start/end line/character of the range.
 */
function makeRange(result: {
    startLine: number
    startCharacter: number
    endLine: number
    endCharacter: number
}): lsp.Range {
    return lsp.Range.create(result.startLine, result.startCharacter, result.endLine, result.endCharacter)
}

/**
 * Compare a position against a range. Returns 0 if the position occurs
 * within the range (inclusive bounds), -1 if the position occurs after
 * it, and +1 if the position occurs before it.
 *
 * @param range The range.
 * @param position The position.
 */
export function comparePosition(range: FlattenedRange, position: lsp.Position): number {
    if (position.line < range.startLine) {
        return +1
    }

    if (position.line > range.endLine) {
        return -1
    }

    if (position.line === range.startLine && position.character < range.startCharacter) {
        return +1
    }

    if (position.line === range.endLine && position.character > range.endCharacter) {
        return -1
    }

    return 0
}
