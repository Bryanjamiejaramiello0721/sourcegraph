import React from 'react'
import { ErrorLike, isErrorLike } from '../../../../../../shared/src/util/errors'
import { RuleTemplate } from '../../form/templates'
import { Markdown } from '../../../../../../shared/src/components/Markdown'
import { renderMarkdown } from '../../../../../../shared/src/util/markdown'

interface Props {
    template: null | RuleTemplate | ErrorLike
}

export const EditCampaignRuleTemplateFormGroupHeader: React.FunctionComponent<Props> = ({ template }) => {
    const TemplateIcon = template !== null && !isErrorLike(template) ? template.icon : undefined

    return template === null ? (
        <div className="alert alert-danger">Invalid campaign template</div>
    ) : isErrorLike(template) ? (
        <div className="alert alert-danger">{template.message}</div>
    ) : !template.isEmpty ? (
        <>
            <h3 className="d-flex align-items-start">
                {TemplateIcon && <TemplateIcon className="icon-inline mr-2 flex-0" />} Edit: {template.title}
            </h3>
            <p>{template.detail && <Markdown dangerousInnerHTML={renderMarkdown(template.detail)} inline={true} />}</p>
        </>
    ) : null
}
