// Command frontend contains the enterprise frontend implementation.
//
// It wraps the open source frontend command and merely injects a few
// proprietary things into it via e.g. blank/underscore imports in this file
// which register side effects with the frontend package.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/authz"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/db"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/hooks"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/shared"
	"github.com/sourcegraph/sourcegraph/cmd/repo-updater/repos"
	_ "github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/auth"
	edb "github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/db"
	iauthz "github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/internal/authz"
	_ "github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/internal/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/internal/licensing"
	_ "github.com/sourcegraph/sourcegraph/enterprise/cmd/frontend/internal/registry"
	"github.com/sourcegraph/sourcegraph/enterprise/pkg/a8n"
	"github.com/sourcegraph/sourcegraph/enterprise/pkg/a8n/resolvers"
	"github.com/sourcegraph/sourcegraph/pkg/conf"
	"github.com/sourcegraph/sourcegraph/pkg/db/dbconn"
	"github.com/sourcegraph/sourcegraph/pkg/db/dbutil"
	"gopkg.in/inconshreveable/log15.v2"
)

func main() {
	initLicensing()
	initAuthz()
	initResolvers()

	hooks.AfterDBInit = func() {
		dsn := conf.Get().ServiceConnections.PostgresDSN
		authzDB, err := dbutil.NewDB(dsn, "frontend-authz")
		if err != nil {
			log.Fatalf("failed to initialize db: %v", err)
		}

		ctx := context.Background()
		go func() {
			t := time.NewTicker(5 * time.Second)
			for range t.C {
				allowAccessByDefault, authzProviders, _, _ :=
					iauthz.ProvidersFromConfig(ctx, conf.Get(), db.ExternalServices, authzDB)
				authz.SetProviders(allowAccessByDefault, authzProviders)
			}
		}()
		go licensing.StartMaxUserCount(&usersStore{})

		syncer := &a8n.ChangesetSyncer{
			Store:       a8n.NewStore(dbconn.Global),
			ReposStore:  repos.NewDBStore(dbconn.Global, sql.TxOptions{}),
			HTTPFactory: repos.NewHTTPClientFactory(),
		}

		go func() {
			if err := syncer.Run(ctx); err != nil {
				log15.Error("ChangesetSyncer.Run", "err", err)
			}
		}()
	}

	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	if debug {
		log.Println("enterprise edition")
	}

	shared.Main()
}

func initLicensing() {
	// Enforce the license's max user count by preventing the creation of new users when the max is
	// reached.
	db.Users.PreCreateUser = licensing.NewPreCreateUserHook(&usersStore{})

	// Make the Site.productSubscription.productNameWithBrand GraphQL field (and other places) use the
	// proper product name.
	graphqlbackend.GetProductNameWithBrand = licensing.ProductNameWithBrand

	// Make the Site.productSubscription.actualUserCount and Site.productSubscription.actualUserCountDate
	// GraphQL fields return the proper max user count and timestamp on the current license.
	graphqlbackend.ActualUserCount = licensing.ActualUserCount
	graphqlbackend.ActualUserCountDate = licensing.ActualUserCountDate

	noLicenseMaximumAllowedUserCount := licensing.NoLicenseMaximumAllowedUserCount
	graphqlbackend.NoLicenseMaximumAllowedUserCount = &noLicenseMaximumAllowedUserCount

	noLicenseWarningUserCount := licensing.NoLicenseWarningUserCount
	graphqlbackend.NoLicenseWarningUserCount = &noLicenseWarningUserCount

	// Make the Site.productSubscription GraphQL field return the actual info about the product license,
	// if any.
	graphqlbackend.GetConfiguredProductLicenseInfo = func() (*graphqlbackend.ProductLicenseInfo, error) {
		info, err := licensing.GetConfiguredProductLicenseInfo()
		if info == nil || err != nil {
			return nil, err
		}
		return &graphqlbackend.ProductLicenseInfo{
			TagsValue:      info.Tags,
			UserCountValue: info.UserCount,
			ExpiresAtValue: info.ExpiresAt,
		}, nil
	}
}

func initAuthz() {
	db.ExternalServices = edb.NewExternalServicesStore()

	// Warn about usage of auth providers that are not enabled by the license.
	graphqlbackend.AlertFuncs = append(graphqlbackend.AlertFuncs, func(args graphqlbackend.AlertFuncArgs) []*graphqlbackend.Alert {
		// Only site admins can act on this alert, so only show it to site admins.
		if !args.IsSiteAdmin {
			return nil
		}

		if licensing.IsFeatureEnabledLenient(licensing.FeatureACLs) {
			return nil
		}

		var authzTypes []string
		ctx := context.Background()

		githubs, err := db.ExternalServices.ListGitHubConnections(ctx)
		if err != nil {
			return []*graphqlbackend.Alert{{
				TypeValue:    graphqlbackend.AlertTypeError,
				MessageValue: fmt.Sprintf("Unable to fetch GitHub external services: %s", err),
			}}
		}
		for _, g := range githubs {
			if g.Authorization != nil {
				authzTypes = append(authzTypes, "GitHub")
				break
			}
		}

		gitlabs, err := db.ExternalServices.ListGitLabConnections(ctx)
		if err != nil {
			return []*graphqlbackend.Alert{{
				TypeValue:    graphqlbackend.AlertTypeError,
				MessageValue: fmt.Sprintf("Unable to fetch GitLab external services: %s", err),
			}}
		}
		for _, g := range gitlabs {
			if g.Authorization != nil {
				authzTypes = append(authzTypes, "GitLab")
				break
			}
		}

		if len(authzTypes) > 0 {
			return []*graphqlbackend.Alert{{
				TypeValue:    graphqlbackend.AlertTypeError,
				MessageValue: fmt.Sprintf("A Sourcegraph license is required to enable repository permissions for the following code hosts: %s. [**Get a license.**](/site-admin/license)", strings.Join(authzTypes, ", ")),
			}}
		}
		return nil
	})

	// Enforce the use of a valid license key by preventing all HTTP requests if the license is invalid
	// (due to a error in parsing or verification, or because the license has expired).
	hooks.PreAuthMiddleware = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info, err := licensing.GetConfiguredProductLicenseInfo()
			if err != nil {
				log15.Error("Error reading license key for Sourcegraph subscription.", "err", err)
				licensing.WriteSubscriptionErrorResponse(w, http.StatusInternalServerError, "Error reading Sourcegraph license key", "Site admins may check the logs for more information. Update the license key in the Sourcegraph management console (https://docs.sourcegraph.com/admin/management_console).")
				return
			}
			if info != nil && info.IsExpiredWithGracePeriod() {
				licensing.WriteSubscriptionErrorResponse(w, http.StatusForbidden, "Sourcegraph license expired", "To continue using Sourcegraph, a site admin must renew the Sourcegraph license (or downgrade to only using Sourcegraph Core features). Update the license key in the Sourcegraph management console.")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func initResolvers() {
	graphqlbackend.NewA8NResolver = resolvers.NewResolver
}

type usersStore struct{}

func (usersStore) Count(ctx context.Context) (int, error) {
	return db.Users.Count(ctx, nil)
}
