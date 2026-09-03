# Zitadel Migration Phase 1 — Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the `mark8ly-admin` and `mark8ly-storefront` Zitadel projects, their OIDC applications, and the credentials and config `auth-bff` will need — with the `restricted` gate reconciled as code, and with zero behaviour change to running mark8ly services.

**Architecture:** All work is declarative config in `tesserix-k8s`, reconciled by the existing `zitadel-bootstrap` CronJob (every 30 min, fails closed on drift). The `zitadel-operator` CRDs are deliberately NOT used here — they issue public PKCE clients only and expose neither `loginVersion` nor `authMethodType`, both of which this design requires. Nothing in this phase is consumed by mark8ly code yet; every new env var is added but unread.

**Tech Stack:** Zitadel v4.15.3 management API, Python 3 (`bootstrap.py`), Helm, ArgoCD, External Secrets Operator, GCP Secret Manager.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md` (in the `mark8ly` repo)

## Global Constraints

- **Execution repo is `tesserix-k8s`**, not `mark8ly`. Only this plan's Task 6 touches `mark8ly`.
- **Branch from `origin/main` after fetching.** `tesserix-k8s` has concurrent sessions; never rewrite someone else's branch.
- **Every chart change needs a `Chart.yaml` version bump** or `ct lint` fails in CI. Local `helm lint` does not catch this.
- **`tesserix-k8s` PRs require a review approval** before merge.
- Zitadel instance: `https://auth.tesserix.app`, v4.15.3. Org `TESSERIX` = `386377229942128837`.
- **In-cluster calls must send `Host: auth.tesserix.app`** — Zitadel resolves the instance by Host header and answers `Instance not found` otherwise.
- **`PUT .../oidc_config` is a full replace, not a patch.** Any write must send every field or it silently resets omitted ones (this reset `authMethodType` and broke every hms login for the life of a PR).
- **protojson elides zero-value fields.** A `false` boolean is absent from responses, not `false`. Never infer "unset" from "absent".
- New config added to mark8ly charts in this phase MUST be optional and unread. A required env var that code rejects when empty causes a crashloop on merge; tests cannot catch it.

---

### Task 1: Teach `zitadel-bootstrap` to reconcile `projectRoleCheck`

The `restricted` gate is the entire structural guarantee of spec decision D2 — it is what stops a storefront customer authenticating against the admin surface. `bootstrap.py` currently does not manage it, so it would be a hand-set flag that nothing asserts and that silently drifts. This task closes that before the flag is ever set.

**Files:**
- Modify: `charts/apps/zitadel-bootstrap/files/bootstrap.py` (in `reconcile_platform_project`, after the `expectedId` check at line ~707)
- Test: `charts/apps/zitadel-bootstrap/files/bootstrap_test.py`
- Modify: `charts/apps/zitadel-bootstrap/Chart.yaml` (version bump)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `platformProjects[].projectRoleCheck` (bool, optional, default `False`) is honoured by the reconciler. Task 3 sets it to `true` for `mark8ly-admin`.

- [ ] **Step 1: Write the failing tests**

Append to `bootstrap_test.py`. That file is **stdlib `unittest` only** — no pytest, no third-party imports — so match its style exactly.

The third test is the important one: it pins the protojson elision behaviour, which is why this cannot be a naive `live.get("projectRoleCheck") == wanted` comparison. The last test pins idempotency, which the file's own docstring names as the property that matters, since the job runs every 30 minutes against a live instance.

```python
class ProjectRoleCheckTest(unittest.TestCase):
    @staticmethod
    def _recording_request(calls, status=200, payload="{}"):
        def _request(method, path, body=None, headers=None):
            calls.append((method, path, body, headers))
            return status, payload
        return _request

    def test_enables_when_desired_and_live_key_is_absent(self):
        """protojson omits projectRoleCheck when false, so absent means OFF."""
        calls = []
        bootstrap.reconcile_project_role_check(
            project_id="1",
            project_name="mark8ly-admin",
            live={"id": "1", "name": "mark8ly-admin"},  # no projectRoleCheck key
            desired=True,
            scope={},
            request=self._recording_request(calls),
        )
        self.assertEqual(len(calls), 1)
        method, path, body, _headers = calls[0]
        self.assertEqual(method, "PUT")
        self.assertEqual(path, "/management/v1/projects/1")
        self.assertIs(body["projectRoleCheck"], True)
        # PUT is a full replace, so name must be resent or it is wiped.
        self.assertEqual(body["name"], "mark8ly-admin")

    def test_no_write_when_already_correct(self):
        calls = []
        bootstrap.reconcile_project_role_check(
            project_id="1",
            project_name="mark8ly-admin",
            live={"id": "1", "name": "mark8ly-admin", "projectRoleCheck": True},
            desired=True,
            scope={},
            request=self._recording_request(calls),
        )
        self.assertEqual(calls, [])

    def test_absent_and_not_desired_is_a_no_op(self):
        calls = []
        bootstrap.reconcile_project_role_check(
            project_id="2",
            project_name="mark8ly-storefront",
            live={"id": "2", "name": "mark8ly-storefront"},
            desired=False,
            scope={},
            request=self._recording_request(calls),
        )
        self.assertEqual(calls, [])

    def test_disables_when_live_is_true_and_not_desired(self):
        calls = []
        bootstrap.reconcile_project_role_check(
            project_id="2",
            project_name="mark8ly-storefront",
            live={"id": "2", "name": "mark8ly-storefront", "projectRoleCheck": True},
            desired=False,
            scope={},
            request=self._recording_request(calls),
        )
        self.assertEqual(len(calls), 1)
        self.assertIs(calls[0][2]["projectRoleCheck"], False)

    def test_unmanaged_when_desired_is_none_and_live_is_true(self):
        """Absent key must not disable a live-true gate. Regression test:
        AgentGateway, AgentRegistry and Atlantis all have projectRoleCheck on
        and declare nothing, so a False default would silently disable them."""
        calls = []
        bootstrap.reconcile_project_role_check(
            project_id="9",
            project_name="AgentGateway",
            live={"id": "9", "name": "AgentGateway", "projectRoleCheck": True},
            desired=None,
            scope={},
            request=self._recording_request(calls),
        )
        self.assertEqual(calls, [])

    def test_unmanaged_when_desired_is_none_and_live_is_absent(self):
        calls = []
        bootstrap.reconcile_project_role_check(
            project_id="9",
            project_name="AgentGateway",
            live={"id": "9", "name": "AgentGateway"},
            desired=None,
            scope={},
            request=self._recording_request(calls),
        )
        self.assertEqual(calls, [])

    def test_raises_on_write_failure(self):
        with self.assertRaises(SystemExit):
            bootstrap.reconcile_project_role_check(
                project_id="1",
                project_name="mark8ly-admin",
                live={"id": "1", "name": "mark8ly-admin"},
                desired=True,
                scope={},
                request=self._recording_request(
                    [], status=403, payload='{"message":"nope"}'
                ),
            )
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd charts/apps/zitadel-bootstrap/files && python3 bootstrap_test.py`
Expected: FAIL with `AttributeError: module 'bootstrap' has no attribute 'reconcile_project_role_check'`

- [ ] **Step 3: Write the minimal implementation**

Add to `bootstrap.py`, above `reconcile_platform_project`:

```python
def reconcile_project_role_check(
    project_id, project_name, live, desired, scope, request=request
):
    """Assert a project's "only authorized users can authenticate" gate.

    protojson omits projectRoleCheck from the response when it is false, so an
    absent key means OFF, not "unset". Comparing with .get(..., False) is what
    makes absent and false the same thing, deliberately.

    The management API has no partial update for a project: PUT replaces the
    whole resource, so name and roleAssertion are resent unchanged.
    """
    # An absent key means UNMANAGED, not False. bootstrap.py reconciles
    # field-by-field so console-set values survive; defaulting to False here
    # would turn OFF the gate on every platformProject that has it on but does
    # not declare it (AgentGateway, AgentRegistry and Atlantis all do today).
    if desired is None:
        return

    current = bool(live.get("projectRoleCheck", False))
    if current == bool(desired):
        return

    body = {
        "name": project_name,
        "projectRoleAssertion": bool(live.get("projectRoleAssertion", False)),
        "projectRoleCheck": bool(desired),
        "hasProjectCheck": bool(live.get("hasProjectCheck", False)),
    }
    status, payload = request(
        "PUT", f"/management/v1/projects/{project_id}", body, headers=scope
    )
    if status != 200:
        raise SystemExit(
            f"project {project_name!r} role-check update failed: {status} {payload!r}"
        )
```

Then call it from `reconcile_platform_project`, immediately after the `expectedId` mismatch check and before `wanted_apps` is read:

```python
    project_id = project["id"]

    reconcile_project_role_check(
        project_id=project_id,
        project_name=desired["name"],
        live=project,
        desired=desired.get("projectRoleCheck"),  # absent => unmanaged, never False
        scope=scope,
    )
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd charts/apps/zitadel-bootstrap/files && python3 bootstrap_test.py`
Expected: `OK`, including all pre-existing tests.

Also run the repo suite the way CI does, since `repo-tests.yaml` drives it through pytest:

Run: `python3 -m pytest charts/apps/zitadel-bootstrap/files/bootstrap_test.py -q`
Expected: all pass.

- [ ] **Step 5: Bump the chart version**

In `charts/apps/zitadel-bootstrap/Chart.yaml`, increment the patch component of `version` (e.g. `0.3.15` → `0.3.16`). CI's `ct lint` fails on an unbumped chart.

- [ ] **Step 6: Commit**

```bash
git add charts/apps/zitadel-bootstrap/files/bootstrap.py \
        charts/apps/zitadel-bootstrap/files/bootstrap_test.py \
        charts/apps/zitadel-bootstrap/Chart.yaml
git commit -m "feat(zitadel-bootstrap): reconcile projectRoleCheck so the restricted gate cannot drift"
```

---

### Task 2: Create the two projects and commit their IDs

`reconcile_platform_project` deliberately does not create projects — it exits with a message telling you to create one and commit its ID, because a project ID is a JWT audience and a changed one must stop reconciliation rather than silently issue tokens the verifier will reject. So creation is a one-time human step.

**This task is executed by a human with instance credentials, not by an agent.** Its output is two IDs.

**Files:**
- No file changes in this task. Task 3 consumes the IDs.

**Interfaces:**
- Consumes: Task 1's reconciler (must be merged first, so the gate is asserted the moment the project exists).
- Produces: two project IDs, referred to below as `<ADMIN_PROJECT_ID>` and `<STOREFRONT_PROJECT_ID>`.

- [ ] **Step 1: Obtain the instance-admin PAT**

```bash
PAT=$(kubectl get secret iam-admin-pat -n zitadel -o jsonpath='{.data.pat}' | base64 -d)
ORG=386377229942128837
```

- [ ] **Step 2: Create `mark8ly-admin`**

```bash
curl -s -X POST https://auth.tesserix.app/management/v1/projects \
  -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  -H "Content-Type: application/json" \
  -d '{"name":"mark8ly-admin","projectRoleAssertion":true,"projectRoleCheck":true,"hasProjectCheck":false}'
```

Expected: `200` with `{"id":"…"}`. Record that id as `<ADMIN_PROJECT_ID>`.

- [ ] **Step 3: Create `mark8ly-storefront`**

```bash
curl -s -X POST https://auth.tesserix.app/management/v1/projects \
  -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  -H "Content-Type: application/json" \
  -d '{"name":"mark8ly-storefront","projectRoleAssertion":true,"projectRoleCheck":false,"hasProjectCheck":false}'
```

Expected: `200` with `{"id":"…"}`. Record that id as `<STOREFRONT_PROJECT_ID>`.

- [ ] **Step 4: Verify the gate reads back as expected**

```bash
curl -s -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  https://auth.tesserix.app/management/v1/projects/<ADMIN_PROJECT_ID>
```

Expected: the response contains `"projectRoleCheck": true`.

```bash
curl -s -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  https://auth.tesserix.app/management/v1/projects/<STOREFRONT_PROJECT_ID>
```

Expected: `projectRoleCheck` is **absent** from the response. That is correct and means `false` — protojson elides it. Do not "fix" this.

- [ ] **Step 5: Record the IDs**

Paste both IDs into the PR description for Task 3. They are not secrets; they are JWT audiences and belong in git.

---

### Task 3: Declare the mark8ly projects, roles and applications

**Files:**
- Modify: `charts/apps/zitadel-bootstrap/values.yaml` (append two entries to `desired.platformProjects`)
- Modify: `charts/apps/zitadel-bootstrap/Chart.yaml` (version bump)

**Interfaces:**
- Consumes: Task 1's `projectRoleCheck` support; Task 2's two project IDs.
- Produces: project role keys `mark8ly.staff` and `mark8ly.customer`, and four OIDC applications. Task 4 grants the login-client machine user; Task 6 consumes the client IDs.

- [ ] **Step 1: Append the two project declarations**

Add to `platformProjects` in `charts/apps/zitadel-bootstrap/values.yaml`, substituting the real IDs from Task 2. `loginBaseUri` points at **our** login page, not Zitadel's hosted UI — that is what makes this a login-client integration. It must be an origin with **no path**: Zitadel appends `/login` itself, and `…/login` produces `…/login/login?authRequest=`.

```yaml
    - org: TESSERIX
      name: mark8ly-admin
      expectedId: "<ADMIN_PROJECT_ID>"
      # Closed product: a user holding no role here cannot obtain a token for
      # it at all. This is what keeps storefront customers out of the merchant
      # admin surface, enforced by Zitadel rather than by our own checks.
      projectRoleCheck: true
      roles:
        - key: mark8ly.staff
          displayName: Merchant Staff
          group: mark8ly-access
      oidcApps:
        - name: mark8ly-admin-web
          appType: OIDC_APP_TYPE_WEB
          authMethodType: OIDC_AUTH_METHOD_TYPE_BASIC
          loginBaseUri: https://admin.mark8ly.com
          redirectUris:
            - https://admin.mark8ly.com/auth/callback
          postLogoutRedirectUris:
            - https://admin.mark8ly.com/
        - name: mark8ly-admin-mobile
          appType: OIDC_APP_TYPE_NATIVE
          authMethodType: OIDC_AUTH_METHOD_TYPE_NONE
          redirectUris:
            - mark8ly-admin:/auth/callback
    - org: TESSERIX
      name: mark8ly-storefront
      expectedId: "<STOREFRONT_PROJECT_ID>"
      # Open product: any TESSERIX user may authenticate. This matches the
      # current open MP-Customer pool; see spec D2 for the accepted risk.
      projectRoleCheck: false
      roles:
        - key: mark8ly.customer
          displayName: Storefront Customer
          group: mark8ly-access
      oidcApps:
        - name: mark8ly-storefront-web
          appType: OIDC_APP_TYPE_WEB
          authMethodType: OIDC_AUTH_METHOD_TYPE_BASIC
          # Per-tenant subdomains cannot each be a login origin, so login runs
          # on the one fixed registered origin and bounces back to the store,
          # reusing the existing /auth/google trampoline shape.
          loginBaseUri: https://mark8ly.com
          redirectUris:
            - https://mark8ly.com/auth/callback
          postLogoutRedirectUris:
            - https://mark8ly.com/
        - name: mark8ly-storefront-mobile
          appType: OIDC_APP_TYPE_NATIVE
          authMethodType: OIDC_AUTH_METHOD_TYPE_NONE
          redirectUris:
            - mark8ly-storefront:/auth/callback
```

- [ ] **Step 2: Verify the values file still parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('charts/apps/zitadel-bootstrap/values.yaml'))" && echo OK`
Expected: `OK`

- [ ] **Step 3: Bump the chart version and lint**

Increment `version` in `charts/apps/zitadel-bootstrap/Chart.yaml`.

Run: `helm lint charts/apps/zitadel-bootstrap`
Expected: `0 chart(s) failed`

- [ ] **Step 4: Commit**

```bash
git add charts/apps/zitadel-bootstrap/values.yaml charts/apps/zitadel-bootstrap/Chart.yaml
git commit -m "feat(zitadel): declare the mark8ly-admin and mark8ly-storefront projects"
```

- [ ] **Step 5: After merge, confirm the reconciler ran clean**

The CronJob runs every 30 minutes. The two confidential web applications must exist before the reconciler asserts them, so if the run fails with `application … is missing`, create them once by hand (Task 4 Step 1) and let the next run assert them.

Run: `kubectl -n zitadel logs job/$(kubectl -n zitadel get jobs -o name | grep zitadel-bootstrap | tail -1 | cut -d/ -f2)`
Expected: completes without `SystemExit`.

---

### Task 4: Mint the `auth-bff` confidential client secret and login-client PAT

Two credentials, both one-time, both outside the reconciler by design — `bootstrap.py`'s own comment records that machine client secrets are deliberately not periodically reconciled.

**This task is executed by a human with instance credentials.**

**Files:**
- Create: `external-secrets/prod/mark8ly/externalsecret.yaml` — append a new `ExternalSecret` (the file already exists and holds `mark8ly-console-catalog`)

**Interfaces:**
- Consumes: Task 3's applications.
- Produces: K8s secret `mark8ly-zitadel` in namespace `mark8ly` with keys `ZITADEL_CLIENT_ID`, `ZITADEL_CLIENT_SECRET`, `ZITADEL_LOGIN_CLIENT_TOKEN`. Task 6 mounts it.

- [ ] **Step 1: Create the two web applications if the reconciler reported them missing**

For each of `mark8ly-admin-web` and `mark8ly-storefront-web`, substituting the project id:

```bash
curl -s -X POST https://auth.tesserix.app/management/v1/projects/<PROJECT_ID>/apps/oidc \
  -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  -H "Content-Type: application/json" \
  -d '{"name":"mark8ly-admin-web","appType":"OIDC_APP_TYPE_WEB","authMethodType":"OIDC_AUTH_METHOD_TYPE_BASIC","grantTypes":["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"],"responseTypes":["OIDC_RESPONSE_TYPE_CODE"],"accessTokenType":"OIDC_TOKEN_TYPE_JWT","redirectUris":["https://admin.mark8ly.com/auth/callback"],"postLogoutRedirectUris":["https://admin.mark8ly.com/"],"loginVersion":{"loginV2":{"baseUri":"https://admin.mark8ly.com"}}}'
```

Expected: `200` with `clientId` and `clientSecret`. **The secret is shown once.** Capture both immediately.

- [ ] **Step 2: Create the login-client machine user**

```bash
curl -s -X POST https://auth.tesserix.app/management/v1/users/machine \
  -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  -H "Content-Type: application/json" \
  -d '{"userName":"mark8ly-login-client","name":"mark8ly login client","description":"Drives the Zitadel Session API v2 for mark8ly auth-bff own login page","accessTokenType":"ACCESS_TOKEN_TYPE_JWT"}'
```

Expected: `200` with `userId`. Note it as `<LOGIN_CLIENT_USER_ID>`.

- [ ] **Step 3: Grant it `IAM_LOGIN_CLIENT` and mint a PAT**

```bash
curl -s -X POST https://auth.tesserix.app/admin/v1/members \
  -H "Authorization: Bearer $PAT" -H "Content-Type: application/json" \
  -d '{"userId":"<LOGIN_CLIENT_USER_ID>","roles":["IAM_LOGIN_CLIENT"]}'

curl -s -X POST https://auth.tesserix.app/management/v1/users/<LOGIN_CLIENT_USER_ID>/pats \
  -H "Authorization: Bearer $PAT" -H "x-zitadel-orgid: $ORG" \
  -H "Content-Type: application/json" -d '{}'
```

Expected: `200` with `token`. **Shown once.**

> This PAT is instance-level. It can check anyone's password and mint a session for any user of any product on this instance. Zitadel offers no narrower role. Treat it as the most powerful credential mark8ly holds, and never log it.

- [ ] **Step 4: Write all three values to GCP Secret Manager**

```bash
for pair in \
  "prod-mark8ly-zitadel-client-id:<CLIENT_ID>" \
  "prod-mark8ly-zitadel-client-secret:<CLIENT_SECRET>" \
  "prod-mark8ly-zitadel-login-client-token:<PAT>"; do
  name="${pair%%:*}"; value="${pair#*:}"
  gcloud secrets create "$name" --project tesseracthub-480811 --replication-policy=automatic 2>/dev/null || true
  printf '%s' "$value" | gcloud secrets versions add "$name" --project tesseracthub-480811 --data-file=-
done
```

- [ ] **Step 5: Add the ExternalSecret**

Append to `external-secrets/prod/mark8ly/externalsecret.yaml`:

```yaml
---
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: mark8ly-zitadel
  namespace: mark8ly
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: gcp-secret-store
    kind: ClusterSecretStore
  target:
    name: mark8ly-zitadel
    creationPolicy: Owner
  data:
    - secretKey: ZITADEL_CLIENT_ID
      remoteRef: {key: prod-mark8ly-zitadel-client-id}
    - secretKey: ZITADEL_CLIENT_SECRET
      remoteRef: {key: prod-mark8ly-zitadel-client-secret}
    - secretKey: ZITADEL_LOGIN_CLIENT_TOKEN
      remoteRef: {key: prod-mark8ly-zitadel-login-client-token}
```

- [ ] **Step 6: Commit**

```bash
git add external-secrets/prod/mark8ly/externalsecret.yaml
git commit -m "feat(mark8ly): provision the Zitadel confidential client and login-client credentials"
```

- [ ] **Step 7: After merge, verify the secret materialised**

Run: `kubectl get secret mark8ly-zitadel -n mark8ly -o jsonpath='{.data}' | tr ',' '\n' | cut -d'"' -f2`
Expected: three keys listed. Never print the values.

---

### Task 5: Prove the two projects behave as designed

The `restricted` gate is the reason for this whole topology, and it fails silently if wrong — a user simply gets in when they should not. Assert it against the real instance before any code depends on it.

**This task is executed by a human with instance credentials. It creates and deletes a throwaway user.**

**Files:**
- Create: `docs/zitadel-mark8ly-verification.md` — record the observed results

**Interfaces:**
- Consumes: Tasks 2, 3 and 4.
- Produces: a recorded verification. No code artefacts.

- [ ] **Step 1: Create a throwaway user with a password and no roles**

```bash
curl -s -X POST https://auth.tesserix.app/v2/users/human \
  -H "Authorization: Bearer $PAT" -H "Content-Type: application/json" \
  -d '{"userId":"verify524NoRoleUserAaBb123","organization":{"orgId":"386377229942128837"},"username":"verify524@mark8ly.test","profile":{"givenName":"Verify","familyName":"NoRole"},"email":{"email":"verify524@mark8ly.test","isVerified":true},"password":{"password":"Ver1fy524!Temp#Pass","changeRequired":false}}'
```

Expected: `200`, and the returned `userId` is **exactly** `verify524NoRoleUserAaBb123`. If Zitadel returned a different, generated id, stop — spec decision D4 does not hold and the later phases must be re-planned.

- [ ] **Step 2: Create a session (password only)**

```bash
LCT=$(kubectl get secret mark8ly-zitadel -n mark8ly -o jsonpath='{.data.ZITADEL_LOGIN_CLIENT_TOKEN}' | base64 -d)
curl -s -X POST https://auth.tesserix.app/v2/sessions \
  -H "Authorization: Bearer $LCT" -H "Content-Type: application/json" \
  -d '{"checks":{"user":{"loginName":"verify524@mark8ly.test"},"password":{"password":"Ver1fy524!Temp#Pass"}}}'
```

Expected: `201` with `sessionId` and `sessionToken`. Record both.

Note: the create response omits `factors`. To see what was actually verified you must re-read the session with `GET /v2/sessions/{id}`; expect `password.verifiedAt` to be present.

- [ ] **Step 3: Confirm the admin project REFUSES a user with no role**

```bash
CID=<mark8ly-admin-web clientId>
CV=verify524codeverifier1234567890abcdefghijk
CC=$(python3 -c "import hashlib,base64;print(base64.urlsafe_b64encode(hashlib.sha256('$CV'.encode()).digest()).decode().rstrip('='))")
LOC=$(curl -s -o /dev/null -D - "https://auth.tesserix.app/oauth/v2/authorize?client_id=$CID&redirect_uri=https%3A%2F%2Fadmin.mark8ly.com%2Fauth%2Fcallback&response_type=code&scope=openid&state=v&code_challenge=$CC&code_challenge_method=S256" | grep -i '^location:' | tr -d '\r' | sed 's/^[Ll]ocation: //')
AR=$(echo "$LOC" | grep -oE 'authRequest(ID)?=[^&]+' | cut -d= -f2)
echo "authRequest: $AR"   # must start with V2_ — if not, loginVersion is not set

curl -s -X POST "https://auth.tesserix.app/v2/oidc/auth_requests/$AR" \
  -H "Authorization: Bearer $LCT" -H "Content-Type: application/json" \
  -d '{"session":{"sessionId":"<SESSION_ID>","sessionToken":"<SESSION_TOKEN>"}}'
```

Expected: **`403` with `Errors.User.GrantRequired`**.

If this returns `200`, the restricted gate is not in force. Stop and fix before proceeding — every later phase assumes this refusal.

- [ ] **Step 4: Confirm the storefront project ADMITS the same user**

Repeat Step 3 with the `mark8ly-storefront-web` client id and its redirect URI, using a fresh auth request.

Expected: `200` with a `callbackUrl` containing `?code=`.

- [ ] **Step 5: Delete the throwaway user and verify it is gone**

```bash
curl -s -X DELETE https://auth.tesserix.app/v2/users/verify524NoRoleUserAaBb123 -H "Authorization: Bearer $PAT"
curl -s -o /dev/null -w '%{http_code}\n' https://auth.tesserix.app/v2/users/verify524NoRoleUserAaBb123 -H "Authorization: Bearer $PAT"
```

Expected: `200` then `404`.

- [ ] **Step 6: Record and commit the results**

Write `docs/zitadel-mark8ly-verification.md` with the date, the two project IDs, and the observed status codes from Steps 1, 3, 4 and 5.

```bash
git add docs/zitadel-mark8ly-verification.md
git commit -m "docs(zitadel): record the mark8ly project isolation verification"
```

---

### Task 6: Add unread Zitadel config to the mark8ly charts

Config lands before any code reads it. A required env var whose validation rejects an empty value crashloops the moment it merges, and no test catches that — so every value here is added while nothing consumes it.

**Files:**
- Modify: `charts/apps/mark8ly-auth-bff/values.yaml` (add a `zitadel:` block alongside the existing `gip:` block)
- Modify: `charts/apps/mark8ly-auth-bff/templates/deployment.yaml` (add env entries)
- Modify: `charts/apps/mark8ly-auth-bff/Chart.yaml` (version bump)

**Interfaces:**
- Consumes: Task 2's project IDs; Task 4's `mark8ly-zitadel` secret.
- Produces: env vars `ZITADEL_ISSUER`, `ZITADEL_ADMIN_PROJECT_ID`, `ZITADEL_STOREFRONT_PROJECT_ID`, `ZITADEL_CLIENT_ID`, `ZITADEL_CLIENT_SECRET`, `ZITADEL_LOGIN_CLIENT_TOKEN` on the `auth-bff` pod. Phase 2 reads them.

- [ ] **Step 1: Add the values block**

In `charts/apps/mark8ly-auth-bff/values.yaml`, alongside the existing `gip:` block:

```yaml
# Zitadel identity. Present but UNREAD until phase 2 lands the login client.
# GIP remains the live provider; nothing here changes behaviour.
zitadel:
  issuer: https://auth.tesserix.app
  adminProjectId: "<ADMIN_PROJECT_ID>"
  storefrontProjectId: "<STOREFRONT_PROJECT_ID>"
  # Credentials come from the mark8ly-zitadel ExternalSecret.
  secretName: mark8ly-zitadel
```

- [ ] **Step 2: Add the env entries**

In `charts/apps/mark8ly-auth-bff/templates/deployment.yaml`, inside the container's `env:` list:

```yaml
            - name: ZITADEL_ISSUER
              value: {{ .Values.zitadel.issuer | quote }}
            - name: ZITADEL_ADMIN_PROJECT_ID
              value: {{ .Values.zitadel.adminProjectId | quote }}
            - name: ZITADEL_STOREFRONT_PROJECT_ID
              value: {{ .Values.zitadel.storefrontProjectId | quote }}
            - name: ZITADEL_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: {{ .Values.zitadel.secretName }}
                  key: ZITADEL_CLIENT_ID
            - name: ZITADEL_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: {{ .Values.zitadel.secretName }}
                  key: ZITADEL_CLIENT_SECRET
            - name: ZITADEL_LOGIN_CLIENT_TOKEN
              valueFrom:
                secretKeyRef:
                  name: {{ .Values.zitadel.secretName }}
                  key: ZITADEL_LOGIN_CLIENT_TOKEN
```

- [ ] **Step 3: Render the template and confirm the env vars appear**

Run: `helm template charts/apps/mark8ly-auth-bff | grep -A2 ZITADEL_ISSUER`
Expected: the rendered env entry with the issuer URL.

- [ ] **Step 4: Lint and bump the chart version**

Increment `version` in `charts/apps/mark8ly-auth-bff/Chart.yaml`.

Run: `helm lint charts/apps/mark8ly-auth-bff`
Expected: `0 chart(s) failed`

- [ ] **Step 5: Commit**

```bash
git add charts/apps/mark8ly-auth-bff/values.yaml \
        charts/apps/mark8ly-auth-bff/templates/deployment.yaml \
        charts/apps/mark8ly-auth-bff/Chart.yaml
git commit -m "feat(mark8ly-auth-bff): land Zitadel config ahead of the login client"
```

- [ ] **Step 6: After merge, confirm the pod still starts**

Run: `kubectl rollout status deploy/mark8ly-auth-bff -n mark8ly --timeout=180s`
Expected: `successfully rolled out`. The new env vars are present and unread; behaviour is unchanged.

---

## Phase 1 completion criteria

- `zitadel-bootstrap` reconciles `projectRoleCheck` and its CronJob completes clean.
- `mark8ly-admin` refuses a role-less user at OIDC finalize with `Errors.User.GrantRequired`; `mark8ly-storefront` admits the same user. Both recorded in `docs/zitadel-mark8ly-verification.md`.
- `POST /v2/users/human` preserves a caller-supplied GIP-shaped `userId` on this instance.
- `mark8ly-auth-bff` runs with Zitadel config present and unread; GIP remains the live provider.

## Remaining phases

Each becomes its own plan, written after the phase before it lands. Phases 2–5 build behind a disabled flag; phase 6 is the single cutover commit, per spec D5 and hms's "two providers at once is the worst state".

| Phase | Scope |
|---|---|
| 2 | `auth-bff` Zitadel login client + `decideSufficiency` (fail-closed, compile-error-to-omit) + archtest, behind a disabled flag |
| 3 | Frontends post credentials to `auth-bff` instead of calling the GIP SDK; storefront trampoline reshaped for Zitadel |
| 4 | `marketplace-api` verifier replacement; drop the `tenant_id` custom claim, resolve tenancy from FGA |
| 5 | `platform-api` `gipadmin` → Zitadel management API; signup via `POST /v2/users/human` |
| 6 | Cutover: flip the flag, recreate the four password accounts, delete `gipkey`, `usermfa`, both GIP verifiers and all GIP config |
