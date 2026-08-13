import type { MessageKey } from "../zh-CN"
import common from "./common"
import iam from "./iam"
import audit from "./audit"
import compute from "./compute"

const enUS = {
  ...common,
  ...iam,
  ...audit,
  ...compute,
} satisfies Record<MessageKey, string>

export default enUS
