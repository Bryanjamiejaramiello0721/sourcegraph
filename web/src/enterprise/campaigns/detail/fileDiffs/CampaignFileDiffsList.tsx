import { LoadingSpinner } from '@sourcegraph/react-loading-spinner'
import H from 'history'
import React from 'react'
import { ExtensionsControllerProps } from '../../../../../../shared/src/extensions/controller'
import * as GQL from '../../../../../../shared/src/graphql/schema'
import { PlatformContextProps } from '../../../../../../shared/src/platform/context'
import { isErrorLike } from '../../../../../../shared/src/util/errors'
import { RepositoryCompareDiffPage } from '../../../../repo/compare/RepositoryCompareDiffPage'
import { ThemeProps } from '../../../../theme'
import { useCampaignFileDiffs } from './useCampaignFileDiffs'
import { FileDiffNode } from '../../../../repo/compare/FileDiffNode'
import { parseRepoURI } from '../../../../../../shared/src/util/url'

interface Props extends ExtensionsControllerProps, PlatformContextProps, ThemeProps {
    campaign: Pick<GQL.ICampaign, 'id'>

    className?: string
    location: H.Location
    history: H.History
}

const LOADING = 'loading' as const

/**
 * A list of files diffs in all changesets in a campaign.
 */
export const CampaignFileDiffsList: React.FunctionComponent<Props> = ({ campaign, className = '', ...props }) => {
    const repositoryComparisons = useCampaignFileDiffs(campaign)
    return (
        <div className={`campaign-file-diffs-list ${className}`}>
            {repositoryComparisons === LOADING ? (
                <LoadingSpinner className="icon-inline mt-3" />
            ) : isErrorLike(repositoryComparisons) ? (
                <div className="alert alert-danger mt-3">{repositoryComparisons.message}</div>
            ) : repositoryComparisons.length === 0 || !repositoryComparisons.some(c => c.fileDiffs.nodes.length > 0) ? (
                <div className="text-muted">No changes</div>
            ) : (
                <div className="card border-left border-right border-top mb-4">
                    {repositoryComparisons.map((c, i) =>
                        c.fileDiffs.nodes.map(d => (
                            <FileDiffNode
                                key={`${i}:${d.internalID}`}
                                {...props}
                                // TODO!(sqs): slice off 'a/' or 'b/' prefixes
                                node={{
                                    ...d,
                                    oldPath: (d.oldPath || '').slice(2),
                                    newPath: (d.newPath || '').slice(2),
                                }}
                                base={{
                                    repoName: c.baseRepository.name,
                                    repoID: c.baseRepository.id,
                                    rev: c.range.baseRevSpec.expr,
                                    commitID: c.range.baseRevSpec.object ? c.range.baseRevSpec.object.oid : '', // TODO!(sqs)
                                }}
                                head={{
                                    repoName: c.headRepository.name,
                                    repoID: c.headRepository.id,
                                    rev: c.range.headRevSpec.expr,
                                    commitID: c.range.headRevSpec.object ? c.range.headRevSpec.object.oid : '', // TODO!(sqs)
                                }}
                                showRepository={true}
                                lineNumbers={false}
                                className="mb-0 border-top-0 border-left-0 border-right-0"
                            />
                        ))
                    )}
                </div>
            )}
        </div>
    )
}
