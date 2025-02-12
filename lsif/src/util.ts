import * as fs from 'mz/fs'
import * as path from 'path'
import { DefinitionReferenceResultId } from './models.database'
import { Id } from 'lsif-protocol'

/**
 * Reads an integer from an environment variable or defaults to the given value.
 *
 * @param key The environment variable name.
 * @param defaultValue The default value.
 */
export function readEnvInt(key: string, defaultValue: number): number {
    return (process.env[key] && parseInt(process.env[key] || '', 10)) || defaultValue
}

/**
 * Determine if an exception value has the given error code.
 *
 * @param e The exception value.
 * @param expectedCode The expected error code.
 */
export function hasErrorCode(e: any, expectedCode: string): boolean {
    return e && e.code === expectedCode
}

/**
 * Return the value of the given key from the given map. If the key does not
 * exist in the map, an exception is thrown with the given error text.
 *
 * @param map The map to query.
 * @param key The key to search for.
 * @param elementType The type of element (used for exception message).
 */
export function mustGet<K, V>(map: Map<K, V>, key: K, elementType: string): V {
    const value = map.get(key)
    if (value !== undefined) {
        return value
    }

    throw new Error(`Unknown ${elementType} '${key}'.`)
}

/**
 * Return the value of the given key from one of the given maps. The first
 * non-undefined value to be found is returned. If the key does not exist in
 * either map, an exception is thrown with the given error text.
 *
 * @param map1 The first map to query.
 * @param map2 The second map to query.
 * @param key The key to search for.
 * @param elementType The type of element (used for exception message).
 */
export function mustGetFromEither<K1, V1, K2, V2>(
    map1: Map<K1, V1>,
    map2: Map<K2, V2>,
    key: K1 & K2,
    elementType: string
): V1 | V2 {
    for (const map of [map1, map2]) {
        const value = map.get(key)
        if (value !== undefined) {
            return value
        }
    }

    throw new Error(`Unknown ${elementType} '${key}'.`)
}

/**
 * Return the value of `id`, or throw an exception if it is undefined.
 *
 * @param id The identifier.
 */
export function assertId<T extends Id>(id: T | undefined): T {
    if (id !== undefined) {
        return id
    }

    throw new Error('id is undefined')
}

/**
 * Hash a string or numeric identifier into the range `[0, maxIndex)`. The
 * hash algorithm here is similar to the one used in Java's String.hashCode.
 *
 * @param id The identifier to hash.
 * @param maxIndex The maximum of the range.
 */
export function hashKey(id: DefinitionReferenceResultId, maxIndex: number): number {
    const s = `${id}`

    let hash = 0
    for (let i = 0; i < s.length; i++) {
        const chr = s.charCodeAt(i)
        hash = (hash << 5) - hash + chr
        hash |= 0
    }

    // Hash value may be negative - must unset sign bit before modulus
    return Math.abs(hash) % maxIndex
}

/**
 * Construct the path of the SQLite database file for the given repository and commit.
 *
 * @param storageRoot The path where SQLite databases are stored.
 * @param repository The repository name.
 * @param commit The repository commit.
 */
export function createDatabaseFilename(storageRoot: string, repository: string, commit: string): string {
    // return path.join(storageRoot, `${encodeURIComponent(repository)}@${commit}.lsif.db`)
    return path.join(storageRoot, `${encodeURIComponent(repository)}.lsif.db`)
}

/**
 * Ensure the directory exists.
 *
 * @param path The directory path.
 */
export async function ensureDirectory(path: string): Promise<void> {
    try {
        await fs.mkdir(path)
    } catch (e) {
        if (!hasErrorCode(e, 'EEXIST')) {
            throw e
        }
    }
}

/**
 * Log an error value to standard error and exit the process after a short
 * timeout to allow outstanding logs to flush.
 *
 * @param e The error value.
 */
export function logErrorAndExit(e: any): void {
    console.error(e)
    setTimeout(() => process.exit(1), 100)
}

/**
 * Determine the table inserter batch size for an entity given the number of
 * fields inserted for that entity. We cannot perform an insert operation with
 * more than 999 placeholder variables, so we need to flush our batch before
 * we reach that amount. If fields are added to the models, the argument to
 * this function also needs to change.
 *
 * @param numFields The number of fields for an entity.
 */
export function getBatchSize(numFields: number): number {
    return Math.floor(999 / numFields)
}
