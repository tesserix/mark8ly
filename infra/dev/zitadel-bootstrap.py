#!/usr/bin/env python3
"""Bootstrap a blank Zitadel into the shape platform-api needs (mark8ly#858).

A fresh instance has an org and a machine user with a PAT, and nothing else.
platform-api's ValidateZitadel refuses to start without an org id, an admin
project id and a login-client token, so CI has to mint the middle two before
any service that talks to Zitadel can come up.

Deliberately NOT a port of tesserix-k8s's zitadel-bootstrap/files/bootstrap.py.
That file is a 1048-line reconciler for a long-lived instance: it manages
branding, SMTP, IDPs and lockout policies, and it explicitly REFUSES to create
a project ("create it once and commit its ID as expectedId"). This initialises
a throwaway instance instead. What is borrowed is its hard-won knowledge of the
API's actual shapes -- that file's header records that every shape is pinned to
what was observed rather than what the docs claim, and the same rule applies
here.

Idempotent: safe to re-run against an instance it already bootstrapped, so a
retried CI step does not fail on "already exists".

Output: writes the three values platform-api needs, in `KEY=value` form, to
the path given by --env-out. It must be written BEFORE platform-api is created,
because compose reads env_file at container-create time.
"""

import argparse
import json
import sys
import time
import urllib.error
import urllib.request

ADMIN_PROJECT = "mark8ly-admin"
ADMIN_OIDC_APP = "mark8ly-admin-web"
# auth-bff's ValidateZitadel requires ZITADEL_APPLE_IDP_ID to be non-empty.
# It is never dereferenced unless a user picks Apple sign-in, which no spec
# does. A real Apple IDP cannot be created here: Zitadel rejects every
# private-key encoding we could synthesise (PKCS8 PEM, SEC1 PEM, raw DER,
# each base64'd) with "Invalid Private Key format (ORG-Fk38d)", and the
# estate's own zitadel-bootstrap.py never creates one either -- it only
# UPDATES a provider made by hand in the console, deliberately withholding
# the .p8. So there is no known-good encoding recorded anywhere to copy.
#
# The placeholder is therefore deliberate and load-bearing only for startup.
# If a spec ever needs Apple sign-in locally, this must become a real IDP
# and that is a research task, not a config change.
APPLE_IDP_PLACEHOLDER = "apple-idp-not-configured-locally"
# Must match platform-api's ZITADEL_STAFF_ROLE_KEY default (pkg/config).
STAFF_ROLE = "mark8ly.staff"


def main() -> int:
    ap = argparse.ArgumentParser()
    # The URL we CONNECT to, which on a developer's machine is a published
    # port and in CI is the same. Distinct from --host below.
    ap.add_argument("--api", default="http://localhost:8092")
    # Zitadel resolves which instance a request is for from the Host header,
    # NOT from the connection. A request to localhost:8092 carrying the
    # wrong Host gets 404 "Instance not found" -- verified against v4.15.3,
    # and the single most confusing failure mode in this file.
    ap.add_argument("--host", default="zitadel:8080")
    ap.add_argument("--org", default="mark8ly")
    ap.add_argument("--pat-file", required=True)
    ap.add_argument("--env-out", required=True)
    ap.add_argument("--admin-redirect", default="http://localhost:4202/login/callback")
    args = ap.parse_args()

    with open(args.pat_file) as fh:
        token = fh.read().strip()
    if not token:
        die(f"{args.pat_file} is empty -- Zitadel mints the PAT only when "
            "ZITADEL_FIRSTINSTANCE_ORG_MACHINE_PAT_EXPIRATIONDATE is set, and "
            "only on the very first start against an empty database")

    api = Zitadel(args.api, args.host, token)
    api.wait_ready()

    org_id = api.org_id(args.org)
    log(f"org {args.org}: {org_id}")

    project_id = api.ensure_project(org_id, ADMIN_PROJECT)
    api.ensure_role(org_id, project_id, STAFF_ROLE)
    client_id, client_secret = api.ensure_oidc_app(
        org_id, project_id, ADMIN_OIDC_APP, args.admin_redirect
    )
    google_idp = api.ensure_google_idp(org_id)

    with open(args.env_out, "w") as fh:
        fh.write(
            "# Written by infra/dev/zitadel-bootstrap.py. Do not edit or commit.\n"
            "ZITADEL_ENABLED=true\n"
            f"ZITADEL_ISSUER=http://{args.host}\n"
            f"ZITADEL_ORG_ID={org_id}\n"
            f"ZITADEL_ADMIN_PROJECT_ID={project_id}\n"
            f"ZITADEL_LOGIN_CLIENT_TOKEN={token}\n"
            f"ZITADEL_ADMIN_CLIENT_ID={client_id}\n"
            f"ZITADEL_ADMIN_CLIENT_SECRET={client_secret}\n"
            f"ZITADEL_ADMIN_REDIRECT_URI={args.admin_redirect}\n"
            f"ZITADEL_GOOGLE_IDP_ID={google_idp}\n"
            f"ZITADEL_APPLE_IDP_ID={APPLE_IDP_PLACEHOLDER}\n"
        )
    log(f"wrote {args.env_out}")
    return 0


class Zitadel:
    def __init__(self, api: str, host: str, token: str) -> None:
        self.api, self.host, self.token = api.rstrip("/"), host, token

    def call(self, method, path, body=None, org_id=None):
        req = urllib.request.Request(
            f"{self.api}{path}",
            data=json.dumps(body).encode() if body is not None else None,
            method=method,
        )
        req.add_header("Host", self.host)
        req.add_header("Authorization", f"Bearer {self.token}")
        req.add_header("Content-Type", "application/json")
        if org_id:
            # The PAT is instance-level; without this the call resolves
            # against the machine user's own org.
            req.add_header("x-zitadel-orgid", org_id)
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return resp.status, json.loads(resp.read() or b"{}")
        except urllib.error.HTTPError as err:
            raw = err.read()
            try:
                return err.code, json.loads(raw or b"{}")
            except json.JSONDecodeError:
                return err.code, {"raw": raw.decode(errors="replace")}

    def wait_ready(self, attempts: int = 60) -> None:
        for _ in range(attempts):
            try:
                req = urllib.request.Request(f"{self.api}/debug/healthz")
                req.add_header("Host", self.host)
                with urllib.request.urlopen(req, timeout=5) as resp:
                    if resp.status == 200:
                        return
            except Exception:  # noqa: BLE001 - any failure means not ready yet
                pass
            time.sleep(3)
        die(f"zitadel did not become ready at {self.api} (Host: {self.host})")

    def org_id(self, name: str) -> str:
        status, body = self.call(
            "POST", "/admin/v1/orgs/_search", {"query": {"limit": 100}}
        )
        if status == 404:
            die(f"404 from {self.api} -- Zitadel resolves the instance from "
                f"the Host header and got {self.host!r}. Check "
                "ZITADEL_EXTERNALDOMAIN/PORT match it exactly.")
        if status != 200:
            die(f"org search failed: {status} {body}")
        for org in body.get("result", []):
            if org["name"] == name:
                return org["id"]
        die(f"org {name!r} not found; ZITADEL_FIRSTINSTANCE_ORG_NAME must match")

    def ensure_project(self, org_id: str, name: str) -> str:
        status, body = self.call(
            "POST", "/management/v1/projects/_search",
            {"query": {"limit": 100}}, org_id=org_id,
        )
        if status != 200:
            die(f"project search failed: {status} {body}")
        existing = next(
            (p for p in body.get("result", []) if p["name"] == name), None
        )
        if existing:
            project_id = existing["id"]
            log(f"project {name}: exists ({project_id})")
        else:
            status, body = self.call(
                "POST", "/management/v1/projects",
                {
                    "name": name,
                    # Puts the granted roles in the token, which is how
                    # platform-api's staff role becomes visible downstream.
                    "projectRoleAssertion": True,
                    # Without a role on this project a user gets
                    # 403 OIDC-foSyH49RvL at FINALIZE -- after a successful
                    # password check, so it reads as broken auth rather than
                    # missing setup.
                    "projectRoleCheck": True,
                    "hasProjectCheck": False,
                },
                org_id=org_id,
            )
            if status != 200:
                die(f"project create failed: {status} {body}")
            project_id = body["id"]
            log(f"project {name}: created ({project_id})")

        # Read back rather than trusting the write. Zitadel's protojson omits
        # false booleans entirely, so a request that silently failed to set
        # projectRoleCheck is indistinguishable from one that set it to false
        # -- and a 200 does not prove the field landed.
        status, body = self.call(
            "POST", "/management/v1/projects/_search",
            {"query": {"limit": 100}}, org_id=org_id,
        )
        live = next(
            (p for p in body.get("result", []) if p["id"] == project_id), None
        )
        if not live or not live.get("projectRoleCheck", False):
            die(f"project {name} has projectRoleCheck=false after bootstrap; "
                "every merchant sign-in would 403 at finalize")
        return project_id

    def ensure_oidc_app(self, org_id, project_id, name, redirect_uri):
        """Mint the admin web app, returning (client_id, client_secret).

        The secret is readable ONLY in the create response -- there is no
        endpoint that returns it later. So on a re-run against an app that
        already exists we regenerate it rather than failing: this instance is
        rebuilt from empty on every CI run, and a stale secret in .env.zitadel
        would fail at sign-in with a generic "bad credentials" rather than
        anything naming the cause.
        """
        status, body = self.call(
            "POST", f"/management/v1/projects/{project_id}/apps/_search",
            {"query": {"limit": 100}}, org_id=org_id,
        )
        if status != 200:
            die(f"app search failed: {status} {body}")
        existing = next(
            (a for a in body.get("result", []) if a.get("name") == name), None
        )
        if existing:
            app_id = existing["id"]
            status, body = self.call(
                "POST",
                f"/management/v1/projects/{project_id}/apps/{app_id}/oidc_config/_generate_client_secret",
                {}, org_id=org_id,
            )
            if status != 200:
                die(f"secret regenerate failed: {status} {body}")
            log(f"oidc app {name}: exists, secret regenerated")
            return existing["oidcConfig"]["clientId"], body["clientSecret"]

        status, body = self.call(
            "POST", f"/management/v1/projects/{project_id}/apps/oidc",
            {
                "name": name,
                "redirectUris": [redirect_uri],
                "responseTypes": ["OIDC_RESPONSE_TYPE_CODE"],
                "grantTypes": [
                    "OIDC_GRANT_TYPE_AUTHORIZATION_CODE",
                    "OIDC_GRANT_TYPE_REFRESH_TOKEN",
                ],
                "appType": "OIDC_APP_TYPE_WEB",
                "authMethodType": "OIDC_AUTH_METHOD_TYPE_BASIC",
                "postLogoutRedirectUris": ["http://localhost:4202/"],
                # http redirect URIs are rejected outright without this.
                "devMode": True,
                "accessTokenType": "OIDC_TOKEN_TYPE_BEARER",
            },
            org_id=org_id,
        )
        if status != 200:
            die(f"oidc app create failed: {status} {body}")
        log(f"oidc app {name}: created ({body['clientId']})")
        return body["clientId"], body["clientSecret"]

    def ensure_google_idp(self, org_id):
        """Create a Google IDP so auth-bff's config validation passes.

        Its credentials are placeholders: nothing signs in with Google in
        CI (sign-in.spec.ts says so explicitly -- the Google popup needs
        real Google Identity Services). Only the ID has to exist.

        Listed via idps/templates/_search, NOT idps/_search: the v1 search
        returns an empty list even when providers exist, which reads as
        "none configured" and makes this create a duplicate on every run.
        """
        status, body = self.call(
            "POST", "/management/v1/idps/templates/_search",
            {"query": {"limit": 100}}, org_id=org_id,
        )
        if status == 200:
            for idp in body.get("result", []):
                if idp.get("name") == "Google":
                    log(f"google idp: exists ({idp['id']})")
                    return idp["id"]
        status, body = self.call(
            "POST", "/management/v1/idps/google",
            {
                "name": "Google",
                "clientId": "e2e-dev-google-client-id",
                "clientSecret": "e2e-dev-google-client-secret",
                "scopes": ["openid", "profile", "email"],
                "providerOptions": {
                    "isLinkingAllowed": True,
                    "isCreationAllowed": True,
                    "isAutoCreation": False,
                    "isAutoUpdate": False,
                },
            },
            org_id=org_id,
        )
        if status != 200:
            die(f"google idp create failed: {status} {body}")
        log(f"google idp: created ({body['id']})")
        return body["id"]

    def ensure_role(self, org_id: str, project_id: str, role_key: str) -> None:
        status, body = self.call(
            "POST", f"/management/v1/projects/{project_id}/roles/_search",
            {"query": {"limit": 100}}, org_id=org_id,
        )
        if status == 200 and any(
            r.get("key") == role_key for r in body.get("result", [])
        ):
            log(f"role {role_key}: exists")
            return
        status, body = self.call(
            "POST", f"/management/v1/projects/{project_id}/roles",
            {"roleKey": role_key, "displayName": "Merchant staff", "group": ""},
            org_id=org_id,
        )
        # An idempotent re-run races here only if two bootstraps run at once;
        # ALREADY_EXISTS is the benign outcome and is not an error.
        if status != 200 and "already exists" not in json.dumps(body).lower():
            die(f"role create failed: {status} {body}")
        log(f"role {role_key}: created")


def log(message: str) -> None:
    print(f"[zitadel-bootstrap] {message}", flush=True)


def die(message: str) -> None:
    print(f"[zitadel-bootstrap] FATAL: {message}", file=sys.stderr, flush=True)
    raise SystemExit(1)


if __name__ == "__main__":
    raise SystemExit(main())
