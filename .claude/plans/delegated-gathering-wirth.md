# Auto-redirect to login on expired JWT (401)

## Context
JWT tokens expire after 24 hours. When this happens, every API call returns "invalid or expired token" but the dashboard shows a raw error with no way to recover. The user must manually navigate to /login. We need to auto-redirect on 401 responses.

## Approach
Handle 401 in the centralized `request()` function in `web/src/lib/api.ts`. When a 401 is received (and the request wasn't to a login/register endpoint), clear the stored token/user and redirect to `/login`.

### File: `web/src/lib/api.ts`
In the `request()` function, after checking `!res.ok` (line 50), add a 401 check **before** throwing the error:

```ts
if (res.status === 401 && !path.startsWith("/v1/auth/")) {
  if (typeof window !== "undefined") {
    localStorage.removeItem("token");
    localStorage.removeItem("user");
    window.location.href = "/login";
  }
}
```

- Guard with `!path.startsWith("/v1/auth/")` so login/register 401s (wrong password) don't redirect
- Guard with `typeof window !== "undefined"` for SSR safety
- Clear localStorage so the auth context picks up the logged-out state on the login page

No other files need changes — the auth context already initializes from localStorage, so clearing it is sufficient.

## Verification
1. Log in, then manually delete the `token` from localStorage (DevTools > Application > Local Storage)
2. Try any dashboard action (list guardrails, save, etc.)
3. Confirm you're redirected to `/login` instead of seeing a raw error
4. Confirm logging in with wrong credentials does NOT redirect (shows error on login page)
5. Rebuild: `docker compose up -d --build web`
