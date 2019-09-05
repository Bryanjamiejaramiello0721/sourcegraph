import { useEffect, useState } from 'react'
import { Observable } from 'rxjs'
import { map } from 'rxjs/operators'
import { dataOrThrowErrors, gql } from '../../../../../../shared/src/graphql/graphql'
import * as GQL from '../../../../../../shared/src/graphql/schema'
import { asError, ErrorLike } from '../../../../../../shared/src/util/errors'
import { queryGraphQL } from '../../../../backend/graphql'
import {
    diffStatFieldsFragment,
    fileDiffFieldsFragment,
    fileDiffHunkRangeFieldsFragment,
} from '../../../../repo/compare/RepositoryCompareDiffPage'
import { gitRevisionRangeFieldsFragment } from '../../../../repo/compare/RepositoryCompareOverviewPage'

const LOADING: 'loading' = 'loading'

export function useCampaignFileDiffs(
    campaign: Pick<GQL.ICampaign, 'id'>
): typeof LOADING | GQL.IRepositoryComparison[] | ErrorLike {
    const [result, setResult] = useState<typeof LOADING | GQL.IRepositoryComparison[] | ErrorLike>(LOADING)
    useEffect(() => {
        const subscription = queryCampaignFileDiffs(campaign).subscribe(setResult, err => setResult(asError(err)))
        return () => subscription.unsubscribe()
    }, [campaign])
    return result
}

function queryCampaignFileDiffs(campaign: Pick<GQL.ICampaign, 'id'>): Observable<GQL.IRepositoryComparison[]> {
    return queryGraphQL(
        gql`
            query CampaignFileDiffs($campaign: ID!) {
                node(id: $campaign) {
                    __typename
                    ... on Campaign {
                        repositoryComparisons {
                            baseRepository {
                                id
                                name
                                url
                            }
                            headRepository {
                                id
                                name
                                url
                            }
                            range {
                                ...GitRevisionRangeFields
                            }
                            fileDiffs {
                                nodes {
                                    ...FileDiffFields
                                }
                                totalCount
                                pageInfo {
                                    hasNextPage
                                }
                                diffStat {
                                    ...DiffStatFields
                                }
                            }
                        }
                    }
                }
            }
            ${gitRevisionRangeFieldsFragment}
            ${fileDiffFieldsFragment}
            ${fileDiffHunkRangeFieldsFragment}
            ${diffStatFieldsFragment}
        `,
        { campaign: campaign.id }
    ).pipe(
        map(dataOrThrowErrors),
        map(data => {
            if (!data || !data.node || data.node.__typename !== 'Campaign') {
                throw new Error('campaign not found')
            }
            return data.node.repositoryComparisons
        })
    )
}
