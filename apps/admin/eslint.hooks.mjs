// The Rules of Hooks gate. Separate from the app's normal lint because
// the shared base config loads eslint-plugin-only-warn, which makes
// every rule in it incapable of failing a build. See the config's doc.
import { reactHooksStrict } from "@repo/eslint-config/react-hooks-strict";
export default reactHooksStrict;
