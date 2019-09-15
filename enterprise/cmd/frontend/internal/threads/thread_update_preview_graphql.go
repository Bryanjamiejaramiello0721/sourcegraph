package threads

import (
	"context"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend/graphqlutil"
)

func NewGQLThreadUpdatePreviewForCreation(input graphqlbackend.CreateThreadInput, repoComparison graphqlbackend.RepositoryComparison) graphqlbackend.ThreadUpdatePreview {
	return &gqlThreadUpdatePreview{new: NewGQLThreadPreview(input, repoComparison)}
}

func NewGQLThreadUpdatePreviewForUpdate(ctx context.Context, old graphqlbackend.Thread, newInput graphqlbackend.CreateThreadInput, newRepoComparison graphqlbackend.RepositoryComparison) (graphqlbackend.ThreadUpdatePreview, error) {
	// Determine if the update will actually change the thread.
	//
	// TODO!(sqs): handle more kinds of changes
	var changed bool
	if old.Title() == newInput.Title {
		changed = true
	}
	if !changed {
		oldRepoComparison, err := old.RepositoryComparison(ctx)
		if err != nil {
			return nil, err
		}
		if equal, err := repoComparisonDiffEqual(ctx, oldRepoComparison, newRepoComparison); err != nil {
			return nil, err
		} else if !equal {
			changed = true
		}
	}

	return &gqlThreadUpdatePreview{old: old, new: NewGQLThreadPreview(newInput, newRepoComparison)}, nil
}

func repoComparisonDiffEqual(ctx context.Context, a, b graphqlbackend.RepositoryComparison) (bool, error) {
	// TODO!(sqs): check all fields
	aDiff, err := a.FileDiffs(&graphqlutil.ConnectionArgs{}).RawDiff(ctx)
	if err != nil {
		return false, err
	}
	bDiff, err := b.FileDiffs(&graphqlutil.ConnectionArgs{}).RawDiff(ctx)
	if err != nil {
		return false, err
	}
	return aDiff == bDiff, nil
}

func NewGQLThreadUpdatePreviewForDeletion(old graphqlbackend.Thread) graphqlbackend.ThreadUpdatePreview {
	return &gqlThreadUpdatePreview{old: old}
}

type gqlThreadUpdatePreview struct {
	old graphqlbackend.Thread
	new graphqlbackend.ThreadPreview
}

func (v *gqlThreadUpdatePreview) OldThread() graphqlbackend.Thread { return v.old }

func (v *gqlThreadUpdatePreview) NewThread() graphqlbackend.ThreadPreview { return v.new }

func (v *gqlThreadUpdatePreview) Operation() graphqlbackend.ThreadUpdateOperation {
	switch {
	case v.old == nil && v.new != nil:
		return graphqlbackend.ThreadUpdateOperationCreation
	case v.old != nil && v.new != nil:
		return graphqlbackend.ThreadUpdateOperationUpdate
	case v.old != nil && v.new == nil:
		return graphqlbackend.ThreadUpdateOperationDeletion
	default:
		panic("unexpected")
	}
}

func (v *gqlThreadUpdatePreview) titleChanged() bool {
	return v.old != nil && v.new != nil && v.old.Title() != v.new.Title()
}

func (v *gqlThreadUpdatePreview) OldTitle() *string {
	if v.titleChanged() {
		return strPtr(v.old.Title())
	}
	return nil
}

func (v *gqlThreadUpdatePreview) NewTitle() *string {
	if v.titleChanged() {
		return strPtr(v.new.Title())
	}
	return nil
}
