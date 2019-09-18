import * as sourcegraph from 'sourcegraph'
import { flatten } from 'lodash'
import { Subscription, Unsubscribable, from } from 'rxjs'
import { toArray } from 'rxjs/operators'
import { memoizedFindTextInFiles } from './util'

export const FIND_REPLACE_REWRITE_COMMAND = 'findReplace.rewrite'

export interface FindReplaceCampaignContext {
    matchTemplate: string
    rule: string | undefined
    rewrite: string
}

export function register(): Unsubscribable {
    const subscriptions = new Subscription()
    setTimeout(() => {
        subscriptions.add(sourcegraph.commands.registerCommand(FIND_REPLACE_REWRITE_COMMAND, rewrite))
        console.log('REG')
    }, 500)
    return subscriptions
}

async function rewrite(context: FindReplaceCampaignContext): Promise<sourcegraph.WorkspaceEdit> {
    const results = flatten(
        await from(
            memoizedFindTextInFiles(
                {
                    pattern: context.matchTemplate,
                    type: 'regexp',
                },
                {
                    repositories: {
                        includes: [],
                        type: 'regexp',
                    },
                    files: {
                        // includes: ['\\.(go|tsx?|java|py)$'], // TODO!(sqs)
                        type: 'regexp',
                    },
                    maxResults: 50, // TODO!(sqs): increase
                }
            )
        )
            .pipe(toArray())
            .toPromise()
    )

    const docs = await Promise.all(results.map(async ({ uri }) => sourcegraph.workspace.openTextDocument(new URL(uri))))

    const edit = new sourcegraph.WorkspaceEdit()
    for (const doc of docs) {
        if (doc.text.length > 15000) {
            continue // TODO!(sqs): skip too large
        }

        // TODO!(sqs): actually implement comby by hitting the api or something
        let i = 0
        while (i !== -1 && i < doc.text.length) {
            i = doc.text.indexOf(context.matchTemplate, i)
            if (i !== -1) {
                const start = doc.positionAt(i)
                const end = doc.positionAt(i + context.matchTemplate.length)
                edit.replace(new URL(doc.uri), new sourcegraph.Range(start, end), context.rewrite)
                i += context.matchTemplate.length
            }
        }
    }

    return edit.toJSON()
}
