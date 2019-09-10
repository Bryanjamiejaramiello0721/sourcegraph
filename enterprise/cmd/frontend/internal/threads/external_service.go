package threads

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/pkg/api"
	"github.com/sourcegraph/sourcegraph/pkg/gitserver"
	"github.com/sourcegraph/sourcegraph/pkg/gitserver/protocol"
)

func CreateOnExternalService(ctx context.Context, threadTitle, threadBody, campaignName string, repo *graphqlbackend.RepositoryResolver, patch []byte) (threadID int64, err error) {
	defaultBranch, err := repo.DefaultBranch(ctx)
	if err != nil {
		return 0, err
	}
	oid, err := defaultBranch.Target().OID(ctx)
	if err != nil {
		return 0, err
	}
	var IsAlphanumericWithPeriod = regexp.MustCompile(`[^a-zA-Z0-9_.]+`)
	branchName := "a8n/" + strings.TrimSuffix(IsAlphanumericWithPeriod.ReplaceAllString(campaignName, "-"), "-") // TODO!(sqs): hack

	// TODO!(sqs): For the prototype, prevent changes to any "live" repositories. The sd9 and sd9org
	// namespaces are sandbox/fake accounts used for the prototype.
	if !strings.HasPrefix(repo.Name(), "github.com/sd9/") && !strings.HasPrefix(repo.Name(), "github.com/sd9org/") {
		return 0, errors.New("refusing to modify non-sd9 test repo")
	}

	// Create a commit and ref.
	refName := "refs/heads/" + branchName
	if _, err := gitserver.DefaultClient.CreateCommitFromPatch(ctx, protocol.CreateCommitFromPatchRequest{
		Repo:       api.RepoName(repo.Name()),
		BaseCommit: api.CommitID(oid),
		TargetRef:  refName,
		Patch:      string(patch),
		CommitInfo: protocol.PatchCommitInfo{
			AuthorName:  "Quinn Slack",         // TODO!(sqs): un-hardcode
			AuthorEmail: "sqs@sourcegraph.com", // TODO!(sqs): un-hardcode
			Message:     "a8n: " + campaignName,
			Date:        time.Now(),
		},
	}); err != nil {
		return 0, err
	}

	// Push the newly created ref. TODO!(sqs) this only makes sense for the demo
	cmd := gitserver.DefaultClient.Command("git", "push", "-f", "--", "origin", fmt.Sprintf("refs/heads/%s:refs/heads/%s", defaultBranch.AbbrevName(), defaultBranch.AbbrevName()), refName+":"+refName)
	cmd.Repo = gitserver.Repo{Name: api.RepoName(repo.Name())}
	if out, err := cmd.CombinedOutput(ctx); err != nil {
		return 0, fmt.Errorf("%s\n\n%s", err, out)
	}

	return createOrGetExistingGitHubPullRequest(ctx, repo.DBID(), repo.DBExternalRepo(), CreateChangesetData{
		BaseRefName: defaultBranch.AbbrevName(),
		HeadRefName: branchName,
		Title:       threadTitle,
		Body:        threadBody + fmt.Sprintf(`\n\n<img src="https://about.sourcegraph.com/sourcegraph-mark.png" width=12 height=12> Campaign: [%s](#)`, campaignName),
	})
}
