package threads

import (
	"context"
	"fmt"

	"github.com/graph-gophers/graphql-go"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/backend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/db"
	"github.com/sourcegraph/sourcegraph/pkg/api"
	"github.com/sourcegraph/sourcegraph/pkg/errcode"
	"github.com/sourcegraph/sourcegraph/pkg/extsvc/github"
	"gopkg.in/inconshreveable/log15.v2"
)

func UpdateGitHubThreadMetadata(ctx context.Context, threadID, threadExternalServiceID int64, externalID string, repoID api.RepoID) error {
	client, externalServiceID, err := getClientForRepo(ctx, repoID)
	if err != nil {
		return err
	}
	if externalServiceID != threadExternalServiceID {
		// TODO!(sqs): handle this case, not sure when it would happen, also is complicated by when
		// there are multiple external services for a repo.  TODO!(sqs): also make this look up the
		// external service using the externalServiceID directly when repo-updater exposes an API to
		// do that.
		return fmt.Errorf("thread %d: external service %d in DB does not match repository external service %d", threadID, threadExternalServiceID, externalServiceID)
	}

	var data struct {
		Node *githubIssueOrPullRequest
	}
	if err := client.RequestGraphQL(ctx, "", `
query($id: ID!) {
	node(id: $id) {
		... on Issue {
`+githubIssueOrPullRequestCommonQuery+`
		}
		... on PullRequest {
`+githubIssueOrPullRequestCommonQuery+`
`+githubPullRequestQuery+`
		}
	}
}
`+githubActorFieldsFragment, map[string]interface{}{
		"id": externalID,
	}, &data); err != nil {
		return err
	}
	if data.Node == nil {
		return fmt.Errorf("github issue or pull request with ID %q not found", externalID)
	}

	externalThread := newExternalThread(data.Node, repoID, externalServiceID)
	return dbUpdateExternalThread(ctx, threadID, externalThread)
}

func createOrGetExistingGitHubIssueOrPullRequest(ctx context.Context, repoID api.RepoID, extRepo api.ExternalRepoSpec, number int32) (threadID int64, err error) {
	client, externalServiceID, err := getClientForRepo(ctx, repoID)
	if err != nil {
		return 0, err
	}

	issue, err := getExistingGitHubIssueOrPullRequest(ctx, client, graphql.ID(extRepo.ID), number)
	if err != nil {
		return 0, err
	}
	return getOrUpdateExistingGitHubIssueOrPullRequest(ctx, repoID, externalServiceID, issue, 0)
}

func getExistingGitHubIssueOrPullRequest(ctx context.Context, client *github.Client, githubRepositoryID graphql.ID, number int32) (*githubIssueOrPullRequest, error) {
	var resp struct {
		Node *struct {
			IssueOrPullRequest *githubIssueOrPullRequest
		}
	}
	if err := client.RequestGraphQL(ctx, "", `
query GetIssueOrPullRequest($repositoryId: ID!, $number: Int!) {
    node(id: $repositoryId) {
        ... on Repository {
            issueOrPullRequest(number: $number)  {
				... on Issue {
`+githubIssueOrPullRequestCommonQuery+`
				}
				... on PullRequest {
`+githubIssueOrPullRequestCommonQuery+`
`+githubPullRequestQuery+`
				}
            }
        }
    }
}
`+githubActorFieldsFragment, map[string]interface{}{
		"repositoryId": githubRepositoryID,
		"number":       number,
	}, &resp); err != nil {
		return nil, err
	}
	if resp.Node == nil {
		return nil, fmt.Errorf("github repository with ID %q not found", githubRepositoryID)
	}
	if resp.Node.IssueOrPullRequest == nil {
		return nil, fmt.Errorf("no github issue or pull request in repository %q with number %d", githubRepositoryID, number)
	}
	return resp.Node.IssueOrPullRequest, nil
}

func createOrGetExistingGitHubThreadsByQuery(ctx context.Context, query string) (threadIDs []int64, err error) {
	// TODO!(sqs): hack, use the first repo that exists to get a handle for the external service, since this query is run globally and not only per-repo.
	repos, err := backend.Repos.List(ctx, db.ReposListOptions{Enabled: true, LimitOffset: &db.LimitOffset{Limit: 1}})
	if err != nil {
		return nil, err
	}
	client, _, err := getClientForRepo(ctx, repos[0].ID)
	if err != nil {
		return nil, err
	}

	extThreads, err := getExistingGitHubThreadsByQuery(ctx, client, query)
	if err != nil {
		return nil, err
	}
	threadIDs = make([]int64, 0, len(extThreads))
	for _, extThread := range extThreads {
		repo, err := backend.Repos.GetByName(ctx, api.RepoName("github.com/"+extThread.Repository.NameWithOwner))
		if err != nil {
			if errcode.IsNotFound(err) {
				log15.Warn("Ignoring issue in repository that is not present on this Sourcegraph instance. Update the external service configuration to include this repository and rerun the import to add this issue.", "repo", extThread.Repository.NameWithOwner, "title", extThread.Title, "number", extThread.Number)
				continue
			}
			return nil, err
		}
		_, externalServiceID, err := getClientForRepo(ctx, repo.ID)
		if err != nil {
			return nil, err
		}
		threadID, err := getOrUpdateExistingGitHubIssueOrPullRequest(ctx, repo.ID, externalServiceID, extThread, 0)
		if err != nil {
			return nil, err
		}
		threadIDs = append(threadIDs, threadID)
	}
	return threadIDs, nil
}

func getExistingGitHubThreadsByQuery(ctx context.Context, client *github.Client, query string) ([]*githubIssueOrPullRequest, error) {
	var resp struct {
		Search struct {
			Nodes []*githubIssueOrPullRequest
		}
	}
	// TODO!(sqs): limited to 100
	if err := client.RequestGraphQL(ctx, "", `
query GitHubThreadsByQuery($query: String!) {
    search(type: ISSUE, first: 100, query: $query) {
		nodes {
			... on Issue {
`+githubIssueOrPullRequestCommonQuery+`
			}
			... on PullRequest {
`+githubIssueOrPullRequestCommonQuery+`
`+githubPullRequestQuery+`
			}
        }
    }
}
`+githubActorFieldsFragment, map[string]interface{}{"query": query}, &resp); err != nil {
		return nil, err
	}
	return resp.Search.Nodes, nil
}
