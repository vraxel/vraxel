import type { Messages } from "../../types"
import common from "./common"
import iam from "./iam"
import audit from "./audit"

const zhCN = {
  ...common,
  ...iam,
  ...audit,
} satisfies Messages

// Every message key in the catalog. zh-CN is the source of truth for the
// key set; en-US must satisfy Record<MessageKey, string> so a missing key
// is a compile error. Orphan en-US keys are caught by the parity test
// (satisfies does not do excess-property checks through spreads).
export type MessageKey = keyof typeof zhCN

// Canary: if any locale file loses its literal keys (e.g. a `: Messages`
// annotation sneaks back in), MessageKey degrades to `string` and the
// en-US Record<MessageKey, string> constraint silently becomes vacuous.
// This alias then fails to compile.
type Assert<T extends true> = T
export type _MessageKeyIsLiteralUnion = Assert<string extends MessageKey ? false : true>

export default zhCN
