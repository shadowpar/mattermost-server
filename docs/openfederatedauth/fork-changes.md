# OpenFederatedAuth Fork Changes

This document summarizes the server-side OpenFederatedAuth work in this Mattermost fork. It starts from the point where the external plugin approach was abandoned in favor of a built-in authentication provider.

## Goals

Mattermost v11 still contains generic OAuth/OIDC plumbing, but the standard login page does not let external plugins add first-class login buttons. This fork adds a built-in OIDC provider so sites can authenticate against arbitrary institutional OIDC providers without depending on Mattermost's paid SSO features or on deprecated GitLab-based workarounds.

The implementation goals are:

- Support multiple OIDC providers under one Mattermost auth service.
- Preserve easy migration from existing GitLab-auth users.
- Validate OIDC ID tokens correctly.
- Keep institution-specific identity mapping claims usable from userinfo.
- Make the login flow visible from the normal Mattermost login and signup pages.

## New Auth Service

The fork adds a new auth service:

```text
openfederatedauth
```

Users authenticated through this flow have:

```text
Users.AuthService = openfederatedauth
Users.AuthData    = configured stable provider claim
```

The provider implementation lives under:

```text
server/channels/app/oauthproviders/openfederatedauth/
```

It is registered from:

```text
server/cmd/mattermost/main.go
```

## Configuration Model

The fork extends Mattermost's public config model with:

```text
OpenFederatedAuthSettings
OpenFederatedAuthProviderSettings
```

The root setting contains global defaults and a `Providers` list. Each provider embeds Mattermost's existing `SSOSettings` shape and adds:

```text
ProviderID
DisplayName
```

The shared SSO settings were also extended with OpenFederatedAuth-specific fields:

```text
EmailClaimName
MattermostIDClaimName
OpenFederatedAuthProviderID
UseProviderIDInAuthData
Issuer
JWKSURI
```

`Issuer` and `JWKSURI` are populated from discovery when possible and used for ID token validation.

## Login UI

The webapp login and signup pages were patched to read the client config value:

```text
OpenFederatedAuthProviders
```

This value is generated server-side from enabled providers. The UI renders one button per provider. The buttons point to:

```text
/oauth/openfederatedauth/login?provider=<provider-id>
/oauth/openfederatedauth/signup?provider=<provider-id>
```

The provider ID is never sent to the OIDC provider as a callback parameter. It is carried inside Mattermost's OAuth state.

## OAuth/OIDC Flow Changes

The fork reuses Mattermost's existing OAuth route structure:

```text
/oauth/openfederatedauth/login
/oauth/openfederatedauth/signup
/signup/openfederatedauth/complete
```

The start route stores these values in state:

```text
action
provider
nonce
token
redirect_to/invite fields when present
```

The authorization request includes:

```text
response_type=code
client_id=<client-id>
redirect_uri=<site-url>/signup/openfederatedauth/complete
state=<mattermost-state>
scope=<configured-scope>
nonce=<generated-nonce>
```

## Discovery

The callback and login-start paths resolve provider endpoints before use. Manual config values are honored first. Missing values are filled from the OIDC discovery document:

```text
authorization_endpoint
token_endpoint
userinfo_endpoint
jwks_uri
issuer
```

This lets operators configure only an issuer/discovery URL for ordinary OIDC providers while retaining manual override support.

## ID Token Validation

The fork validates OpenFederatedAuth ID tokens before trusting their claims.

Validation includes:

- JWKS signature verification.
- Allowed signing methods: RS256/384/512, PS256/384/512, ES256/384/512.
- `iss` matches configured/discovered issuer.
- `aud` contains the configured client ID.
- `exp` is valid.
- `iat` is valid.
- `nonce` matches the nonce stored in Mattermost state.

The implementation uses the existing repo dependency:

```text
github.com/golang-jwt/jwt/v5
```

## Userinfo Role

The validated ID token is the primary trust source. Userinfo is still called and used for missing or site-local claims.

This matters because providers often expose local mapping attributes only from userinfo. For example, a Keycloak mapper may expose:

```text
mattermostid -> id
```

only in userinfo, not in the ID token.

The auth data resolution order is:

1. Configured `MattermostIDClaimName` from the validated ID token.
2. Configured `MattermostIDClaimName` from userinfo.
3. `sub` from the validated ID token.

This keeps OIDC validation strong while allowing local account mapping policies.

## Account Mapping

The fork uses Mattermost's normal external-auth storage model rather than adding a parallel identity database.

Login first searches by:

```text
AuthService = openfederatedauth
AuthData    = resolved stable claim
```

If that is missing, creation/mapping checks the email address. If the email exists and the provider determines it is the same external user, the auth data is updated and the existing user is logged in. If the email exists but auth data differs, the login is rejected.

This behavior preserves the safety property users expect from the old GitLab workaround: same email alone is not enough to silently take over an existing account when the stable external identity differs.

## Migration Rationale

Many sites used GitLab auth as a practical bridge to arbitrary OIDC/OAuth2 providers. Those users already have meaningful Mattermost values in:

```text
Users.AuthService = gitlab
Users.AuthData    = stable institutional ID
```

The fork intentionally does not namespace `AuthData` by provider by default. This permits a direct migration:

```text
gitlab -> openfederatedauth
```

while preserving the existing `AuthData` value.

It also permits multiple OpenFederatedAuth providers, such as CILogon and Keycloak, to map to the same Mattermost user when they emit the same stable ID and email.

## User Limit Patch

The fork disables the unlicensed user-limit banner/enforcement added in newer Mattermost releases by setting the user limit constants to zero. This is independent of OpenFederatedAuth, but it is part of the operational fork because the deployment target is self-hosted open-source Mattermost without a commercial license.

The relevant file is:

```text
server/channels/app/limits.go
```

## Security Review Notes

Reviewers should pay particular attention to:

- OIDC discovery endpoint resolution.
- JWKS key selection by `kid` and `alg`.
- ID token validation options.
- Nonce generation, state storage, and callback checking.
- Account mapping behavior when email matches but auth data differs.
- Whether local mapping claims are emitted in ID token, userinfo, or both.

The implementation does not store a separate identity map outside Mattermost's normal `Users.AuthService` and `Users.AuthData` fields.

## Known Limits

- This is OIDC-only. Generic OAuth2 providers that do not return ID tokens are not supported.
- JWKS keys are fetched during login validation and are not cached yet.
- Userinfo is still required by the resolver unless discovery provides it; this is intentional for local mapping claims and profile enrichment.
- The current implementation supports standard public RSA and EC JWKs used by common OIDC providers.
