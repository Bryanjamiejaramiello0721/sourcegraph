package a8n

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/sourcegraph/pkg/a8n"
	"github.com/sourcegraph/sourcegraph/pkg/db/dbtest"
	"github.com/sourcegraph/sourcegraph/pkg/extsvc/github"
)

var dsn = flag.String("dsn", "", "Database connection string to use in integration tests")

func TestStore(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	d, cleanup := dbtest.NewDB(t, *dsn)
	defer cleanup()

	tx, done := dbtest.NewTx(t, d)
	defer done()

	now := time.Now().UTC().Truncate(time.Microsecond)
	s := NewStoreWithClock(tx, func() time.Time {
		return now.UTC().Truncate(time.Microsecond)
	})

	ctx := context.Background()

	t.Run("Campaigns", func(t *testing.T) {
		campaigns := make([]*a8n.Campaign, 0, 3)

		t.Run("Create", func(t *testing.T) {
			for i := 0; i < cap(campaigns); i++ {
				c := &a8n.Campaign{
					Name:         fmt.Sprintf("Upgrade ES-Lint %d", i),
					Description:  "All the Javascripts are belong to us",
					AuthorID:     23,
					ChangesetIDs: []int64{int64(i) + 1},
				}

				if i%2 == 0 {
					c.NamespaceOrgID = 23
				} else {
					c.NamespaceUserID = 42
				}

				want := c.Clone()
				have := c

				err := s.CreateCampaign(ctx, have)
				if err != nil {
					t.Fatal(err)
				}

				if have.ID == 0 {
					t.Fatal("ID should not be zero")
				}

				want.ID = have.ID
				want.CreatedAt = now
				want.UpdatedAt = now

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}

				campaigns = append(campaigns, c)
			}
		})

		t.Run("Count", func(t *testing.T) {
			count, err := s.CountCampaigns(ctx, CountCampaignsOpts{})
			if err != nil {
				t.Fatal(err)
			}

			if have, want := count, int64(len(campaigns)); have != want {
				t.Fatalf("have count: %d, want: %d", have, want)
			}

			count, err = s.CountCampaigns(ctx, CountCampaignsOpts{ChangesetID: 1})
			if err != nil {
				t.Fatal(err)
			}

			if have, want := count, int64(1); have != want {
				t.Fatalf("have count: %d, want: %d", have, want)
			}
		})

		t.Run("List", func(t *testing.T) {
			for i := 1; i <= len(campaigns); i++ {
				opts := ListCampaignsOpts{ChangesetID: int64(i)}

				ts, next, err := s.ListCampaigns(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}

				if have, want := next, int64(0); have != want {
					t.Fatalf("opts: %+v: have next %v, want %v", opts, have, want)
				}

				have, want := ts, campaigns[i-1:i]
				if len(have) != len(want) {
					t.Fatalf("listed %d campaigns, want: %d", len(have), len(want))
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatalf("opts: %+v, diff: %s", opts, diff)
				}
			}

			for i := 1; i <= len(campaigns); i++ {
				cs, next, err := s.ListCampaigns(ctx, ListCampaignsOpts{Limit: i})
				if err != nil {
					t.Fatal(err)
				}

				{
					have, want := next, int64(0)
					if i < len(campaigns) {
						want = campaigns[i].ID
					}

					if have != want {
						t.Fatalf("limit: %v: have next %v, want %v", i, have, want)
					}
				}

				{
					have, want := cs, campaigns[:i]
					if len(have) != len(want) {
						t.Fatalf("listed %d campaigns, want: %d", len(have), len(want))
					}

					if diff := cmp.Diff(have, want); diff != "" {
						t.Fatal(diff)
					}
				}
			}

			{
				var cursor int64
				for i := 1; i <= len(campaigns); i++ {
					opts := ListCampaignsOpts{Cursor: cursor, Limit: 1}
					have, next, err := s.ListCampaigns(ctx, opts)
					if err != nil {
						t.Fatal(err)
					}

					want := campaigns[i-1 : i]
					if diff := cmp.Diff(have, want); diff != "" {
						t.Fatalf("opts: %+v, diff: %s", opts, diff)
					}

					cursor = next
				}
			}
		})

		t.Run("Update", func(t *testing.T) {
			for _, c := range campaigns {
				c.Name += "-updated"
				c.Description += "-updated"
				c.AuthorID++

				if c.NamespaceUserID != 0 {
					c.NamespaceUserID++
				}

				if c.NamespaceOrgID != 0 {
					c.NamespaceOrgID++
				}

				now = now.Add(time.Second)
				want := c
				want.UpdatedAt = now

				have := c.Clone()
				if err := s.UpdateCampaign(ctx, have); err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}

				// Test that duplicates are not introduced.
				have.ChangesetIDs = append(have.ChangesetIDs, have.ChangesetIDs...)
				if err := s.UpdateCampaign(ctx, have); err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}

				// Test we can add to the set.
				have.ChangesetIDs = append(have.ChangesetIDs, 42)
				want.ChangesetIDs = append(want.ChangesetIDs, 42)

				if err := s.UpdateCampaign(ctx, have); err != nil {
					t.Fatal(err)
				}

				sort.Slice(have.ChangesetIDs, func(a, b int) bool {
					return have.ChangesetIDs[a] < have.ChangesetIDs[b]
				})

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}

				// Test we can remove from the set.
				have.ChangesetIDs = have.ChangesetIDs[:0]
				want.ChangesetIDs = want.ChangesetIDs[:0]

				if err := s.UpdateCampaign(ctx, have); err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}
			}
		})

		t.Run("Get", func(t *testing.T) {
			t.Run("ByID", func(t *testing.T) {
				want := campaigns[0]
				opts := GetCampaignOpts{ID: want.ID}

				have, err := s.GetCampaign(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}
			})

			t.Run("NoResults", func(t *testing.T) {
				opts := GetCampaignOpts{ID: 0xdeadbeef}

				_, have := s.GetCampaign(ctx, opts)
				want := ErrNoResults

				if have != want {
					t.Fatalf("have err %v, want %v", have, want)
				}
			})
		})

		t.Run("Delete", func(t *testing.T) {
			for i := range campaigns {
				err := s.DeleteCampaign(ctx, campaigns[i].ID)
				if err != nil {
					t.Fatal(err)
				}

				count, err := s.CountCampaigns(ctx, CountCampaignsOpts{})
				if err != nil {
					t.Fatal(err)
				}

				if have, want := count, int64(len(campaigns)-(i+1)); have != want {
					t.Fatalf("have count: %d, want: %d", have, want)
				}
			}
		})
	})

	t.Run("Changesets", func(t *testing.T) {
		githubActor := github.Actor{
			AvatarURL: "https://avatars2.githubusercontent.com/u/1185253",
			Login:     "mrnugget",
			URL:       "https://github.com/mrnugget",
		}
		githubPR := &github.PullRequest{
			ID:           "FOOBARID",
			Title:        "Fix a bunch of bugs",
			Body:         "This fixes a bunch of bugs",
			URL:          "https://github.com/sourcegraph/sourcegraph/pull/12345",
			Number:       12345,
			Author:       githubActor,
			Participants: []github.Actor{githubActor},
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		changesets := make([]*a8n.Changeset, 0, 3)

		t.Run("Create", func(t *testing.T) {
			for i := 0; i < cap(changesets); i++ {
				th := &a8n.Changeset{
					RepoID:              42,
					CreatedAt:           now,
					UpdatedAt:           now,
					Metadata:            githubPR,
					CampaignIDs:         []int64{int64(i) + 1},
					ExternalID:          fmt.Sprintf("foobar-%d", i),
					ExternalServiceType: "github",
				}

				changesets = append(changesets, th)
			}

			err := s.CreateChangesets(ctx, changesets...)
			if err != nil {
				t.Fatal(err)
			}

			for _, have := range changesets {
				if have.ID == 0 {
					t.Fatal("id should not be zero")
				}

				want := have.Clone()

				want.ID = have.ID
				want.CreatedAt = now
				want.UpdatedAt = now

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}
			}
		})

		t.Run("Count", func(t *testing.T) {
			count, err := s.CountChangesets(ctx, CountChangesetsOpts{})
			if err != nil {
				t.Fatal(err)
			}

			if have, want := count, int64(len(changesets)); have != want {
				t.Fatalf("have count: %d, want: %d", have, want)
			}

			count, err = s.CountChangesets(ctx, CountChangesetsOpts{CampaignID: 1})
			if err != nil {
				t.Fatal(err)
			}

			if have, want := count, int64(1); have != want {
				t.Fatalf("have count: %d, want: %d", have, want)
			}
		})

		t.Run("List", func(t *testing.T) {
			for i := 1; i <= len(changesets); i++ {
				opts := ListChangesetsOpts{CampaignID: int64(i)}

				ts, next, err := s.ListChangesets(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}

				if have, want := next, int64(0); have != want {
					t.Fatalf("opts: %+v: have next %v, want %v", opts, have, want)
				}

				have, want := ts, changesets[i-1:i]
				if len(have) != len(want) {
					t.Fatalf("listed %d changesets, want: %d", len(have), len(want))
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatalf("opts: %+v, diff: %s", opts, diff)
				}
			}

			for i := 1; i <= len(changesets); i++ {
				ts, next, err := s.ListChangesets(ctx, ListChangesetsOpts{Limit: i})
				if err != nil {
					t.Fatal(err)
				}

				{
					have, want := next, int64(0)
					if i < len(changesets) {
						want = changesets[i].ID
					}

					if have != want {
						t.Fatalf("limit: %v: have next %v, want %v", i, have, want)
					}
				}

				{
					have, want := ts, changesets[:i]
					if len(have) != len(want) {
						t.Fatalf("listed %d changesets, want: %d", len(have), len(want))
					}

					if diff := cmp.Diff(have, want); diff != "" {
						t.Fatal(diff)
					}
				}
			}

			{
				ids := make([]int64, len(changesets))
				for i := range changesets {
					ids[i] = changesets[i].ID
				}

				have, _, err := s.ListChangesets(ctx, ListChangesetsOpts{IDs: ids})
				if err != nil {
					t.Fatal(err)
				}

				want := changesets
				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}
			}

			{
				var cursor int64
				for i := 1; i <= len(changesets); i++ {
					opts := ListChangesetsOpts{Cursor: cursor, Limit: 1}
					have, next, err := s.ListChangesets(ctx, opts)
					if err != nil {
						t.Fatal(err)
					}

					want := changesets[i-1 : i]
					if diff := cmp.Diff(have, want); diff != "" {
						t.Fatalf("opts: %+v, diff: %s", opts, diff)
					}

					cursor = next
				}
			}
		})

		t.Run("Get", func(t *testing.T) {
			t.Run("ByID", func(t *testing.T) {
				want := changesets[0]
				opts := GetChangesetOpts{ID: want.ID}

				have, err := s.GetChangeset(ctx, opts)
				if err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(have, want); diff != "" {
					t.Fatal(diff)
				}
			})

			t.Run("NoResults", func(t *testing.T) {
				opts := GetChangesetOpts{ID: 0xdeadbeef}

				_, have := s.GetChangeset(ctx, opts)
				want := ErrNoResults

				if have != want {
					t.Fatalf("have err %v, want %v", have, want)
				}
			})
		})

		t.Run("Update", func(t *testing.T) {
			want := make([]*a8n.Changeset, 0, len(changesets))
			have := make([]*a8n.Changeset, 0, len(changesets))

			now = now.Add(time.Second)
			for _, c := range changesets {
				c.Metadata = []byte(`{"updated": true}`)
				c.ExternalServiceType = "gitlab"

				if c.RepoID != 0 {
					c.RepoID++
				}

				have = append(have, c.Clone())

				c.UpdatedAt = now
				want = append(want, c)
			}

			if err := s.UpdateChangesets(ctx, have...); err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(have, want); diff != "" {
				t.Fatal(diff)
			}

			for i := range have {
				// Test that duplicates are not introduced.
				have[i].CampaignIDs = append(have[i].CampaignIDs, have[i].CampaignIDs...)
			}

			if err := s.UpdateChangesets(ctx, have...); err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(have, want); diff != "" {
				t.Fatal(diff)
			}

			for i := range have {
				// Test we can add to the set.
				have[i].CampaignIDs = append(have[i].CampaignIDs, 42)
				want[i].CampaignIDs = append(want[i].CampaignIDs, 42)
			}

			if err := s.UpdateChangesets(ctx, have...); err != nil {
				t.Fatal(err)
			}

			for i := range have {
				sort.Slice(have[i].CampaignIDs, func(a, b int) bool {
					return have[i].CampaignIDs[a] < have[i].CampaignIDs[b]
				})

				if diff := cmp.Diff(have[i], want[i]); diff != "" {
					t.Fatal(diff)
				}
			}

			for i := range have {
				// Test we can remove from the set.
				have[i].CampaignIDs = have[i].CampaignIDs[:0]
				want[i].CampaignIDs = want[i].CampaignIDs[:0]
			}

			if err := s.UpdateChangesets(ctx, have...); err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(have, want); diff != "" {
				t.Fatal(diff)
			}
		})
	})
}
