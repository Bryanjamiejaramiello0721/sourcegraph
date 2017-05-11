import * as take from "lodash/take";
import * as React from "react";
import { Link } from "react-router";
import URI from "vs/base/common/uri";
import { tokenizeToString } from "vs/editor/common/modes/textToHtmlTokenizer";
import { IModeService } from "vs/editor/common/services/modeService";

import { urlToBlob } from "sourcegraph/blob/routes";
import { ChevronRight } from "sourcegraph/components/symbols/Primaries";
import { colors } from "sourcegraph/components/utils";
import { Services } from "sourcegraph/workbench/services";
import { getURIContext } from "sourcegraph/workbench/utils";

interface Props {
	results?: GQL.ISearchResults;
	loading: boolean;
}
const resultsSx = {

};

export function ResultsView(props: Props): JSX.Element {
	if (props.loading) {
		return <div style={resultsSx}>
			Loading
		</div>;
	}

	if (!props.results) {
		return <div style={resultsSx} />;
	}
	if (props.results.results.length === 0) {
		return <div style={resultsSx}>
			No results found.
		</div>;
	}
	const truncated = props.results.results.length > 100;
	const results = take(props.results.results, 100);
	return <div style={resultsSx}>
		{results.map(FileResult)}
		{truncated && <Truncated />}
	</div>;
}

function Truncated(): JSX.Element {
	return <div style={{ textAlign: "center", marginTop: 30, marginBottom: 100 }}>
		Results truncated. Refine your search to view other results.
	</div>;
}

const codeSx = {
	backgroundColor: "white",
	borderRadius: 2,
	display: "flex",
	overflow: "scroll",
	maxWidth: "100%",
};

const nuSx = {
	backgroundColor: colors.blueGrayL1(),
	margin: 0,
	padding: "0 5px",
	borderRadius: "2px 0 0 2px",
	marginRight: 10,
	textAlign: "right",
};

const fileMatchSx = {
	marginBottom: 20,
	marginTop: 5,
};

function FileResult(fileMatch: GQL.IFileMatch, key: number): JSX.Element {
	const truncated = fileMatch.lineMatches.length > 5;
	const matches = take(fileMatch.lineMatches, 5);
	return <div style={fileMatchSx} key={key}>
		<File resource={fileMatch.resource} />
		<div style={codeSx} >
			<pre style={nuSx}>
				{matches.map(LineNumber)}
			</pre>
			<pre style={{ margin: 0 }}>
				{matches.map((line, i) => LineMatch(fileMatch.resource, line, i))}
			</pre>
		</div>
		{truncated && <MoreResults resource={fileMatch.resource} />}
	</div>;
}

function MoreResults(props: { resource: string }): JSX.Element {
	const { repo, rev, path } = getURIContext(URI.parse(props.resource));
	return <Link to={urlToBlob(repo, rev, path)}>
		<div style={{ marginTop: 5, textAlign: "center" }}>
			More results in this file <ChevronRight />
		</div>
	</Link>;
}

function File(props: { resource: string }): JSX.Element {
	const { repo, rev, path } = getURIContext(URI.parse(props.resource));
	return <div>
		<Link style={{ padding: "5px 0" }} to={urlToBlob(repo, rev, path)}>
			{repo} — {path}
		</Link>
	</div>;
}

function LineNumber(match: GQL.ILineMatch, key: number): JSX.Element {
	return <span key={key}>
		{match.lineNumber + 1}
		<br />
	</span>;
}

function LineMatch(resource: string, match: GQL.ILineMatch, key: number): JSX.Element {
	const ref = async (span) => {
		if (!span) {
			return;
		}
		const text = match.preview.replace(/^\W+/, "");
		// TODO(nicot): This should block until the mode is ready, but it
		// doesn't wait for the language tokenizer properly.
		const modeService = Services.get(IModeService) as IModeService;
		const mode = await modeService.getModeIdByFilenameOrFirstLine(resource);
		await modeService.getOrCreateMode(mode);
		const node = document.createElement("div");
		node.innerHTML = `<div class="code">${tokenizeToString(text, mode)}</div>`;
		span.appendChild(node);
	};
	return <span key={key} ref={ref} />;
}
