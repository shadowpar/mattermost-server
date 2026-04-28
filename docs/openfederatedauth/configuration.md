# OpenFederatedAuth Configuration

OpenFederatedAuth is this fork's built-in OIDC authentication provider for Mattermost. It supports one or more OIDC providers, renders one login button per configured provider, validates ID tokens using OIDC discovery/JWKS, and maps users into Mattermost by email plus a configured stable identity claim.

The auth service name stored in Mattermost users is:

```text
openfederatedauth
```

## Login URLs

For a single provider, users normally start from the Mattermost login page button. The direct login URL is:

```text
https://mattermost.example.org/oauth/openfederatedauth/login?provider=<provider-id>
```

The callback URL registered with each OIDC provider is:

```text
https://mattermost.example.org/signup/openfederatedauth/complete
```

The `provider` value is not included in the callback URL. Mattermost stores it in the OAuth/OIDC `state` value and recovers it when the provider redirects back.

## Minimal Configuration

This example uses discovery for both providers. Secrets are omitted here.

```json
"OpenFederatedAuthSettings": {
  "Enable": true,
  "ButtonText": "Open Federated Auth",
  "ButtonColor": "#145DBF",
  "Providers": [
    {
      "ProviderID": "cilogon",
      "DisplayName": "CILogon",
      "Enable": true,
      "Id": "mattermost-client-id",
      "Secret": "mattermost-client-secret",
      "DiscoveryEndpoint": "https://cilogon.org/bnlsdcc",
      "Scope": "openid profile email org.cilogon.userinfo",
      "EmailClaimName": "email",
      "MattermostIDClaimName": "mattermostid"
    },
    {
      "ProviderID": "sdcc-keycloak",
      "DisplayName": "SDCC Keycloak",
      "Enable": true,
      "Id": "mattermost-client-id",
      "Secret": "mattermost-client-secret",
      "DiscoveryEndpoint": "https://auth.sdcc.bnl.gov/auth/realms/chat",
      "Scope": "openid profile email",
      "EmailClaimName": "email",
      "MattermostIDClaimName": "id"
    }
  ]
}
```

## Provider Fields

`ProviderID`
: Required when using `Providers`. This is the stable local identifier used in Mattermost login URLs, for example `sdcc-keycloak`. Use lowercase URL-safe names and avoid changing this after deployment.

`DisplayName`
: Optional but recommended. This is the label shown on the login/signup button.

`Enable`
: Required for active providers. Disabled providers are not shown on the login page and cannot be used.

`Id`
: Required. The OIDC client ID.

`Secret`
: Required for confidential clients using the current code flow implementation.

`DiscoveryEndpoint`
: Recommended. May be either the issuer/base URL or the full `.well-known/openid-configuration` URL. If the value does not contain `/.well-known/`, Mattermost appends `/.well-known/openid-configuration`.

`AuthEndpoint`
: Optional when discovery is configured. Manual authorization endpoint override.

`TokenEndpoint`
: Optional when discovery is configured. Manual token endpoint override.

`UserAPIEndpoint`
: Optional when discovery is configured. Manual userinfo endpoint override.

`Issuer`
: Optional when discovery is configured. Manual issuer override used to validate ID tokens.

`JWKSURI`
: Optional when discovery is configured. Manual JWKS endpoint override used to validate ID token signatures.

`Scope`
: Optional. Defaults to `openid profile email`. Must include `openid`; otherwise the provider may not return an ID token.

`EmailClaimName`
: Optional. Defaults to `email`. Used to find the user's email address. If the claim is multivalued, the first non-empty value is used.

`MattermostIDClaimName`
: Optional. Defaults to `sub`. This should be a stable, non-reassignable identifier from the provider. It becomes Mattermost `AuthData`. Existing GitLab migrations can use the same claim value that was previously stored as GitLab `AuthData`.

`UseProviderIDInAuthData`
: Optional. Defaults to `false`. When true, Mattermost stores `AuthData` as `<provider-id>:<claim-value>`. Leave this false if multiple providers should map to the same Mattermost user when they emit the same stable identifier.

`ButtonText`
: Optional. Provider-level login button label fallback. `DisplayName` is preferred when present.

`ButtonColor`
: Optional. Provider-level login button color. Falls back to the root `OpenFederatedAuthSettings.ButtonColor`.

## Discovery Behavior

When `DiscoveryEndpoint` is configured, Mattermost fetches the OIDC metadata document and fills any missing endpoint fields from:

```text
authorization_endpoint
token_endpoint
userinfo_endpoint
jwks_uri
issuer
```

Manual values win. For example, you can configure only `DiscoveryEndpoint` and `TokenEndpoint`; Mattermost will keep your manual token endpoint and discover the others.

If discovery is not configured, these values must be present manually:

```text
AuthEndpoint
TokenEndpoint
UserAPIEndpoint
Issuer
JWKSURI
```

## Identity Claim Precedence

Mattermost validates the ID token first, then builds the user identity using this order:

1. Configured `MattermostIDClaimName` from the validated ID token.
2. Configured `MattermostIDClaimName` from userinfo.
3. `sub` from the validated ID token.

This supports providers such as Keycloak where a local mapping claim, for example `id` from `mattermostid`, is exposed only from userinfo while the ID token still provides the trusted OIDC assertion.

Profile fields such as `given_name`, `family_name`, `name`, and `preferred_username` are read from the validated ID token first, with userinfo used to fill missing fields.

## Account Mapping

On login, Mattermost first looks for an existing user by:

```text
AuthService = openfederatedauth
AuthData    = resolved MattermostIDClaimName value
```

If no user is found by auth data, Mattermost checks for a user with the same email address.

If the email exists and the provider determines the external identity is the same user, Mattermost updates that user's auth data and logs them in. If the email exists but the auth data does not match, login is rejected to avoid taking over another account.

For migrations from GitLab, the intended simple path is to update existing Mattermost users from:

```text
AuthService = gitlab
AuthData    = existing stable mattermostid value
```

to:

```text
AuthService = openfederatedauth
AuthData    = same existing stable mattermostid value
```

## Callback Registration

Each OIDC provider must allow this exact redirect URI:

```text
https://mattermost.example.org/signup/openfederatedauth/complete
```

The per-provider selector is carried in Mattermost state, not in the callback URL.

## Notes

- OpenFederatedAuth is OIDC-only. It is not intended to support generic OAuth2 providers that do not issue ID tokens.
- ID tokens are validated with JWKS signature verification, issuer, audience, expiration, issued-at, and nonce checks.
- Userinfo remains important for site-local claims that are not included in ID tokens.
- The login and signup pages show one OpenFederatedAuth button per enabled provider.
