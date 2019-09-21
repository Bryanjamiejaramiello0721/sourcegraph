// Package confdefaults contains default configuration files for various
// deployment types.
//
// It is a separate package so that users of pkg/conf do not indirectly import
// pkg/db/confdb, which we have a linter to protect against.
package confdefaults

import "github.com/sourcegraph/sourcegraph/pkg/conf/conftypes"

// TODO(slimsag): consider moving these into actual json files for improved
// editing.

// DevAndTesting is the default configuration applied to dev instances of
// Sourcegraph, as well as what is used by default during Go testing.
//
// Tests that wish to use a specific configuration should use conf.Mock.
//
// Note: This actually generally only applies to 'go test' because we always
// override this configuration via *_CONFIG_FILE environment variables.
var DevAndTesting = conftypes.RawUnified{
	Critical: `{
	"auth.providers": [
		{
			"type": "builtin",
			"allowSignup": true
		}
	],
}`,
	Site: `{
	"search.index.enabled": true,
}`,
}

// DockerContainer is the default configuration applied to Docker
// single-container instances of Sourcegraph.
var DockerContainer = conftypes.RawUnified{
	Critical: `{
	"auth.providers": [
		{
			"type": "builtin",
			"allowSignup": true
		}
	]
}`,
	Site: `{
	"disablePublicRepoRedirects": true,
	"search.index.enabled": true,
}`,
}

// Cluster is the default configuration applied to Cluster instances of
// Sourcegraph.
var Cluster = conftypes.RawUnified{
	Critical: `{
	// Publicly accessible URL to web app (e.g., what you type into your browser).
	"externalURL": "http://localhost:3080",

	// The authentication provider to use for identifying and signing in users.
	// Only one entry is supported.
	//
	// The builtin auth provider with signup disallowed (shown below) means that
	// after the initial site admin signs in, all other users must be invited.
	//
	// Other providers are documented at https://docs.sourcegraph.com/admin/auth.
	"auth.providers": [
		{
			"type": "builtin",
			"allowSignup": false
		}
	]
}`,
	Site: `{
	"search.index.enabled": true,
}`,
}

// Default is the default for *this* deployment type. It is populated by
// pkg/conf at init time.
var Default conftypes.RawUnified
