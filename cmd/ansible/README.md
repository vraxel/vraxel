# cmd/ansible — 真机 E2E 驱动

`lib/ansible` 的测试套件覆盖了本地连接器能到达的一切。但**测试机不是目标机**：SSH 传输、sudo 对含引号路径的包裹、systemd、以及各模块真实发出的 shell 命令，进程内测不了。这个命令补的就是这一段。

## 用法

对真机跑完整套件（默认就指向 `e2e/site.yml`）：

```bash
go run ./cmd/ansible -host 10.1.1.10 -user root -password secret
```

密钥认证、非默认端口：

```bash
go run ./cmd/ansible -host 10.1.1.10 -port 2222 -user ops -key ~/.ssh/id_ed25519
```

多主机，用 JSON inventory：

```bash
go run ./cmd/ansible -inventory hosts.json
```

跑任意别的 playbook（`-dir` 同时是 `roles/` 的查找根）：

```bash
go run ./cmd/ansible -dir path/to/project -playbook site.yml -host 10.1.1.10
```

### 多主机套件

单主机跑不出来的语义（分组解析、组变量 vs 主机变量、`serial` 分批、`run_once`、跨主机 `delegate_to`、**每主机各自求值的循环**）在 `multi.yml`：

```bash
cp cmd/ansible/e2e/hosts.example.json /tmp/hosts.json   # 填自己的主机
go run ./cmd/ansible -playbook multi.yml -inventory /tmp/hosts.json -var password=...
```

示例 inventory 里的口令是占位符，用 `-var` 覆盖即可，不必改文件。三台主机各带一份**不同长度**的 `host_packages`，断言各自创建的文件数——这正是「循环按主机求值」的回归测试，单主机永远测不出来。

跨主机事实靠控制端汇总：每台主机通过 `delegate_to: localhost` 往控制端文件追加一行，最后一个 play 读回来断言总数。这是 playbook 观察「别的主机发生了什么」的唯一办法。

> 套件拆成三个 play 是有原因的：`serial` 会让**整个 play（含 pre_tasks/post_tasks）每批各跑一次**（与 Ansible 一致），所以「重置计数」和「断言总数」不能放在同一个 play 里。`run_once` 同理是**每批一次**，不是每次运行一次。

### PTY 模式

`-pty` 让 SSH 连接器走 PTY 分支（并挂一个 live writer，否则该分支不会激活）：

```bash
go run ./cmd/ansible -host 10.1.1.10 -user root -password secret -pty
```

这条路径是 vraxel-server 做交互式执行时走的。加它是因为它此前从未被真实跑过——第一次跑就发现 PTY 的 `\r\n` 会残留在 register 值里，让所有字符串比较静默失败。

### 本地冒烟（无需远程主机）

除去必须真机的任务外，整套剧本可以用本地连接器跑：

```bash
go run ./cmd/ansible -host localhost -var connection=local -become=false -skip-tag remote-only
```

带 `remote-only` 标签的是 systemd 单元的装/起/停/删和端口探测，其余（循环语义、file/stat 的带引号路径、environment、changed_when、模板过滤器、delegate_to、block/rescue/always、until/retries）本地就能验完。

## inventory JSON

```json
{
  "hosts": {
    "10.1.1.12": {"node_role": "primary"},
    "10.1.1.13": {"node_role": "replica"}
  },
  "groups": {"db": {"hosts": ["10.1.1.12", "10.1.1.13"]}},
  "vars": {"remote_user": "root", "password": "secret"}
}
```

主机变量优先于 inventory 变量。`connection=ssh`、`become=true`、`pty=false` 在 inventory 未设置时自动补上。

> `pty=false` 是有意的：PTY 路径在没有挂 live writer 时，短命令偶发丢输出（见 `connector/ssh` 的注释），而这里的输出是要拿去断言的。

## E2E 套件覆盖什么

`e2e/` 下每个任务都**自带断言**，跑绿代表引擎行为正确，而不只是跑完了。

| 面 | 位置 |
|---|---|
| `gather_facts`、role 的 defaults/vars 优先级、role 内 `set_fact` 外溢 | `site.yml` pre_tasks + `roles/e2e_role` |
| `loop` 模板求值、`with_items` 展开、`with_dict` 配对、`loop_control`、空循环跳过 | `tasks/loops.yml`（按产出文件数断言） |
| `file` 的 directory/touch/link/absent + mode、`stat` 的 JSON 与缺失路径语义 | `tasks/modules.yml`（路径**故意带空格和单引号**） |
| `environment` 的普通值/模板值/含引号值 | `tasks/modules.yml` |
| `changed_when` 两个方向 | `tasks/modules.yml` |
| `template` 模块 + `selectattr`/`rejectattr`/`mapattr`/`flatten` | `templates/report.conf` |
| systemd 单元装/起/停/删（自建一次性单元，`always` 清理） | `site.yml`，标签 `remote-only` |
| `wait_for` 端口与超时 | `site.yml` |
| `delegate_to`（play 走 SSH，delegate 走本地，跨连接器类型） | `site.yml` |
| `block`/`rescue`/`always`、`until`/`retries` | `site.yml` |
| 分组解析、组变量 vs 主机变量、`serial` 分批、`run_once`、跨主机 `delegate_to`、每主机各自求值的循环、多 play | `multi.yml` |

套件产生的东西全在 `/tmp/vraxel-e2e` 加一个一次性 systemd 单元，post_tasks 会删干净并断言删掉了。

## 为什么还有 `e2e_test.go`

套件只有在有人拿真机跑时才会执行，否则里面的笔误要到那天才暴露。`e2e_test.go` 在 `make check` 里就把它钉住：剧本能解析、能通过引擎同一套加载期校验、只引用已注册的模块、模板渲染结果与断言里写的字符串一致。它已经抓到过一次 YAML 引号写错。
