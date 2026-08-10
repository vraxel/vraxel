import type { MessageKey } from "../zh-CN"
import common from "./common"
import iam from "./iam"
import audit from "./audit"

const enUS = {
  ...common,
  ...iam,
  ...audit,
} satisfies Record<MessageKey, string>

export default enUS
