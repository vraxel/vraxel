# CLAUDE.md

## 原则

「以第一性原理！从原始需求和问题本质出发，不从惯例或模板出发。
1. 不要假设我清楚自己想要什么。动机或目标不清晰时，停下来讨论。
2. 目标清晰但路径不是最短的，直接告诉我并建议更好的办法。
3. 遇到问题追根因，不打补丁。每个决策都要能回答"为什么"。
4. 输出说重点，砍掉一切不改变决策的信息。」
5. 永远不要假设，先看实际代码。
6. 不要为了迎合我而出迎合我的方案，永远拿出长期、根本性、彻底、干净的最佳方案，不要让我每次问有没有更好的方案，就又拿出更好的方案，直接想好了然后告诉我最好的！

## Approach
1. Think before acting. Read existing files before writing code.
2. Be concise in output but thorough in reasoning.
3. Prefer editing to rewriting whole files.
4. Do not re-read files you have already read unless the file may have changed.
5. Test your code before declaring done.
6. No sycophantic openers or closing fluff.
7. Keep solutions simple and direct.
8. User instructions always override this file.
9. 干净、根本性的改动优先于最小、最快捷的改动。

## Rules

- **禁止整条 lint 规则降级。** 一条规则要么在 `error`，要么不启用；**任何规则都不许停在 `warn`**。`ui` 的 lint 脚本带 `--max-warnings 0`，`warn` 对 CI 等价于 `error`，降级换不来绿灯，只换来看不见的债——种子仓库(lcp)就是这么攒出 ~340 处违规而 CI 全程绿灯的（`eslint .` 对 warning 退出码是 0，已实测：同样 3 个 warning，`eslint .` 退出 0，`eslint . --max-warnings 0` 退出 1）。需要例外时，在**出问题的那一行**写 `// eslint-disable-next-line <rule>` 并在上一行写明理由：它出现在 diff 里、grep 得到、随被豁免的代码一起消失。整条降级三样都不占。
- **规则严重性决策的源头在 lcp 的 `eslint.shared.js`**，本仓 `ui/eslint.config.js` 顶部持有一份标注为 MIRROR 的副本（没有私有 npm registry，无法真共享包）。改任何一边都要同步另一边。本仓的 config 曾经原样抄来 lcp 的降级块——而本仓对那些规则**一处违规都没有**。规则决策靠复制粘贴传播，就会在两个地方同时是错的。
- **新规则族只能以 `error` 引入，否则就先别引入。** 升级 lint 插件大版本（如 `eslint-plugin-react-hooks` v7 带进来的 React Compiler 规则族）时，要么当场把违规清零并置于 `error`，要么把新规则关掉、留 issue 排期。以 `warn` 引入等于引入一笔没有负责人、没有期限的债；更麻烦的是**这类规则按组件只报一次**（一处 `Compilation Skipped` 会掩盖同组件其余全部问题），所以修的过程中总数会**先涨后降**，warning 计数既不能当进度也做不了 ratchet，唯一稳定状态是零。
- 所有代码变动，都要新建一个 worktree（分支），在 worktree 上改动后等我 Review 确定再合并；清理 worktree 时同步清理掉分支。
- If a new feature requires adding new API endpoints, **MUST confirm with the user before implementation**. Do not create new routes, resources, or API groups without explicit approval.
- When designing any concurrency/locking mechanism, **MUST account for vraxel-server horizontal scaling** (multiple instances). In-process locks (`sync.Mutex`, etc.) are insufficient — use distributed locks (e.g., PostgreSQL advisory locks) since all instances share the same database.
- The `docs/` directory is **local-only by convention** (excluded via `.gitignore: docs/*`). When writing design / plan / feature docs, save them locally as usual but **do NOT commit them, and do NOT ask the user whether to commit them**.
- **All git commit messages MUST be in English.** Body can include supplementary details in Chinese if needed, but the subject line must be English.
- **Database migration files MUST be created via `make new-migration NAME=description_here`**，自动生成 `YYYYMMDDHHMMSS_{name}.up.sql`。禁止手动创建文件 -- 8-digit `YYYYMMDD` 会导致同日多个 migration 版本冲突。
- **新增任何"可在列表/详情查看"的资源，MUST 一致展示创建人(created_by)**。① 建表即加可空 `created_by bigint` + FK `REFERENCES users(id) ON DELETE SET NULL`；② 所有创建路径用 `oidc.UserIDFromContext(ctx)` 写入，多个 INSERT 入口逐一接通（漏一处=该入口创建的资源永远查不到创建人）；③ Get + 全部 List 查询用 `LEFT JOIN users u ON u.id = <表>.created_by` + `COALESCE(NULLIF(u.display_name,''), u.username,'') AS creator_name`（统一此单字段约定；LEFT 不 INNER，否则删用户/NULL 会丢行；若该表已 JOIN users 取 owner，创建人另起别名如 `cu`）；④ API 只读字段 `createdByName`，跑 `make generate`；⑤ 前端列表"创建人"列放"创建时间"右侧、详情加"创建人"行，复用 i18n `common.createdBy`，值 `|| "-"` 兜底，骨架屏/空状态列数同步 +1。**禁止前端调 `iam:users:list` 自行解析名字**（越权，破坏跨模块边界——名字解析必须在各资源自己的 SQL JOIN 内完成）。
- **Layer architecture**: `pkg/apis` follows a three-layer onion rule documented in `pkg/apis/ARCHITECTURE.md` (top-level -> business -> store -> pkg/db). `pkg/db` and its subpackages (incl. `generated`) are store-layer-only; business / handler code uses `lib/list` for paginated list semantics. Run `./scripts/check-layer-leak.sh` (part of `make check`) before commit. New modules must follow the per-module refactor checklist in that document.
- **禁止前端任何形式的"动态加载"** —— 不允许 `React.lazy()`、`<Suspense>` 用于路由/页面/弹窗的代码分割，不允许 react-router `lazy:` 路由字段，不允许 `await import("...")` / 动态 `import()` 表达式。所有模块必须 ESM 静态 `import`。理由：动态加载（1）与 `<BrowserRouter> + useRoutes()` 声明式路由不兼容，`lazy:` 字段会被静默忽略导致页面渲染空白；（2）切 tab / 打开弹窗时引入一次性 chunk 下载延迟，把单次 build 时间换成每个用户的首次加载延迟；（3）混用 static + dynamic import 同一文件触发 vite `INEFFECTIVE_DYNAMIC_IMPORT`。文件过大需要切 chunk 时**拆文件而不是包 lazy**。仅 HTML 原生 `<img loading="lazy">`、`<iframe loading="lazy">` 不在禁止范围。

### Output
- Return code first. Explanation after, only if non-obvious.
- No inline prose. Use comments sparingly - only where logic is unclear.
- No boilerplate unless explicitly requested.
- 所有输出使用中文

### Code Rules
- Simplest working solution. No over-engineering.
- No abstractions for single-use operations.
- No speculative features or "you might also want..."
- Read the file before modifying it. Never edit blind.
- No docstrings or type annotations on code not being changed.
- No error handling for scenarios that cannot happen.
- Three similar lines is better than a premature abstraction.

### Review Rules
- State the bug. Show the fix. Stop.
- No suggestions beyond the scope of the review.
- No compliments on the code before or after the review.

### Debugging Rules
- Never speculate about a bug without reading the relevant code first.
- State what you found, where, and the fix. One pass.
- If cause is unclear: say so. Do not guess.

### Simple Formatting
- No em dashes, smart quotes, or decorative Unicode symbols.
- Plain hyphens and straight quotes only.
- Natural language characters (accented letters, CJK, etc.) are fine when the content requires them.
- Code output must be copy-paste safe.

## Dev Environment

### 启动

`make dev` 同时起后端 `vraxel-server`（`:9099`）与 vite（`:5199`，HMR）。开发配置 `app/vraxel-server/config.dev.yaml`（gitignored 覆盖层，存在时优先于 committed `config.yaml`）。

**Worktree**: `config.dev.yaml` 和 `ui/dist/`（内嵌前端资源，均 gitignored）不会带进新 worktree，创建后手动复制：

```bash
cp app/vraxel-server/config.dev.yaml .worktrees/<branch>/app/vraxel-server/config.dev.yaml
cp -r ui/dist .worktrees/<branch>/ui/dist
```

**禁止在 worktree 里跑 `pnpm install` / `pnpm typecheck` / `pnpm build`**。worktree 只用来改前端源码；改完 commit → merge 回 main 后，**在 main 上跑 `pnpm typecheck`**（main 已有 `node_modules`，0 成本）。理由：worktree 各装一份 `node_modules` 数百 MB，pnpm 跨 worktree 共享会被 `.modules.yaml` 的 `virtualStoreDir` 打架；把 typecheck 推到 merge 之后是「一处装、一处验」。同理**禁止用 symlink 把 worktree 的 `ui/node_modules` 指向 main**。

**例外：跨模块的大批量机械重构（一次动 20+ 文件的 codemod / 规则清理 / 符号搬迁）直接在主检出上开分支做，不要用 worktree。** 这类改动每一批都需要立刻 typecheck 才能收敛——盲改到 merge 才发现错误，会把「改一批、验一批」变成「改六十个文件、一次性面对几十个报错」。主检出有 `node_modules`，边改边验是 0 成本。做完照常等 Review 再合。

**这类重构的 codemod 必须基于 AST（`ts-morph` / `jscodeshift`），不许用正则或按行切。** 正则切不动多行 `import {}` 和跨行箭头函数体，会静默产出「看着对」的残缺文件；lcp 那次就切坏了 5 个文件，全靠 `tsc` 才捞回来。

### 生成物与校验

- `make generate` 刷新三套 committed 产物（顺序：sqlc → OpenAPI → tygo TS 类型），改完 SQL/`pkg/apis` 类型后必跑并提交结果。
- `make check` 是提交前所有闸门：gofmt -s、go vet、layer guard、Go tests、UI typecheck/lint/tests。触及并发代码时再跑更深的 `go test -race ./...`。
- `make fmt` 格式化 Go 与 UI。

### API E2E Testing

生成 dev JWT（admin，长 TTL）：

```bash
TOKEN=$(go run ./cmd/dev-token)
curl -s --cookie "vraxel_at=$TOKEN" http://localhost:9099/api/...
```

Auth 是 cookie-only（BFF 模式），cookie 为 `vraxel_at` / `vraxel_rt` / `vraxel_csrf`。缺少 `vraxel_csrf` cookie 时跳过 CSRF 校验，故 curl 仅带 `vraxel_at` 即可发 POST/PUT/DELETE。

**端口约束**：vraxel dev 用 `:9099`/`:5199` 以与 lcp 共存。浏览器 cookie 同源策略不区分 port，同一 hostname 多端口跑多实例会共用同一份 auth cookie，第二次登录覆盖第一次会话并触发 token refresh race。本地并行多套环境时，用不同 hostname（在 `/etc/hosts` 加 `dev.vraxel.local` 等）而非仅换端口。

## API Architecture

REST 资源在 `pkg/apis/<module>/install.go` 通过 `rest.APIGroupInfo` 声明（三层 onion，详见 `pkg/apis/ARCHITECTURE.md`）。URL 路径与权限码由框架自动派生，不要手写。

**URL**: `/api/{GroupName}/{Version}/{Resource.Name}`。资源 `Name` 用复数 kebab-case，绝不把模块名塞进 Name（用 `Name: "users"`，不是 `Name: "iam-users"`），Group 名已提供前缀。

**Permission**: `{module}:{resource}:{verb}`，按 Storage 实现的接口自动检测：`Lister`->`list`、`Getter`->`get`、`Creator`->`create`、`Updater`->`update`、`Patcher`->`patch`、`Deleter`->`delete`、`CollectionDeleter`->`deleteCollection`。

**四种资源类型**（按语义选，不按方便选）：

| 类型 | 用途 | URL | 权限 |
|---|---|---|---|
| `Resources[i]` | 顶层 CRUD | `/{name}` | 从 Storage 接口自动派生 |
| `SubResources[i]` | 父级下的嵌套 CRUD | `/{parent}/{id}/{name}` | 自有权限树，自动 |
| `CustomVerbs[i]` | 对某条目的只读视图 | `/{parent}/{id}:{verb}` | 自动继承父级 `get` |
| `Actions[i]` | 有副作用的变更 | `/{parent}/{id}/{verb}` | 必须显式 `PermissionTargets`（否则 panic） |

- 只读单条查询 -> CustomVerb（零权限样板）。
- 嵌套 CRUD 资源 -> SubResource（自动权限树）。
- 变更/副作用动词 -> Action（必须声明 `PermissionTargets`）。
- 只读查询绝不要用 Action。

**Scope（Platform / Workspace / Namespace）** 由注册深度决定：顶层 `Resources` = Platform；`workspaces` 下嵌套 = Workspace；`workspaces/namespaces` 下嵌套 = Namespace。`workspaces` / `namespaces` 段会被 `rbac_sync` 从权限码剥离（如 `GET /api/iam/v1/workspaces/1/...` 检查的是不含 `workspaces` 段的权限码）。仅管理员功能只注册在 Platform scope。

**Cross-Module**: Go 包不得 import 另一模块的 REST handler；前端也不得为满足自己页面的数据需求去调另一模块的端点。模块 A 需要模块 B 的数据时，A 在自己包内建只读 proxy Storage（借用 A 自己的权限树 via `PermissionTargets`），返回 A 自己的 API 类型。这样操作者只需 A 的权限即可用 A 的页面。

新增模块时按 `pkg/apis/ARCHITECTURE.md` 的 New Module Checklist 执行，完成后跑 `./scripts/check-layer-leak.sh` 应零告警。
