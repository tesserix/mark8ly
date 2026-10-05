// The Rules of Hooks gate. Separate from eslint.config.mjs because the
// shared base loads eslint-plugin-only-warn, which makes every rule in
// that config incapable of failing a build. See the config's own doc.
import { reactHooksStrict } from "@repo/eslint-config/react-hooks-strict";
export default reactHooksStrict;
