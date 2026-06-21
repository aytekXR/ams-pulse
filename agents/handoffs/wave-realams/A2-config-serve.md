# A2 — config + serve wiring for AMS login credentials

**Scope (single writer):** `server/cmd/pulse/config.go`, `server/cmd/pulse/serve.go`.
Do NOT touch `server/pkg/amsclient` (that is A1's scope). **Author only — do NOT `git add`/commit.**

A1 is adding two fields to `amsclient.Config`: `LoginEmail string` and `LoginPassword string`.
Your job is to read two new env vars and pass them into `amsclient.New`. Field names must match A1's
exactly: **`LoginEmail`**, **`LoginPassword`**.

## config.go

1. In the `EnvConfig` struct, next to the existing AMS fields (`AMSBaseURL`, `AMSNodeID`,
   `AMSAuthToken`, `AMSApplications`, around lines 37–47), add:
   ```go
   AMSLoginEmail    string // PULSE_AMS_LOGIN_EMAIL — AMS console email for cookie-session auth
   AMSLoginPassword string // PULSE_AMS_LOGIN_PASSWORD — AMS console password
   ```
2. In `loadEnvConfig`, right after the `AMSAuthToken: os.Getenv("PULSE_AMS_AUTH_TOKEN"),` line
   (~line 155), add — matching the house style for secrets (bare `os.Getenv`, no default):
   ```go
   AMSLoginEmail:    os.Getenv("PULSE_AMS_LOGIN_EMAIL"),
   AMSLoginPassword: os.Getenv("PULSE_AMS_LOGIN_PASSWORD"),
   ```

## serve.go

In the `amsclient.New(amsclient.Config{...})` literal (~lines 130–134), add the two new fields:
```go
amsClient := amsclient.New(amsclient.Config{
    BaseURL:       cfg.AMSBaseURL,
    AuthToken:     cfg.AMSAuthToken,
    LoginEmail:    cfg.AMSLoginEmail,
    LoginPassword: cfg.AMSLoginPassword,
    Timeout:       10 * time.Second,
})
```

Do not change any other behaviour. If config.go has a unit test that snapshots the env surface, update
it to include the two new vars (search `config_test.go` for `PULSE_AMS_AUTH_TOKEN`; mirror that pattern
if present — otherwise no test change is needed).

Return: a concise summary of the exact edits made (file:line) and whether a config test needed updating.
