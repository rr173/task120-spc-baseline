# task120-spc

一个统计过程控制（SPC）质量监控引擎：创建控制图（单值-移动极差 I-MR /
均值-极差 X-bar&R / 不合格品率 p-chart），按序提交测量点，按确定公式重算
控制限（CL/UCL/LCL，±1σ/±2σ/±3σ 分区），用 Westgard 多规则
（1-2s/1-3s/2-2s/R-4s/4-1s/10-x）按时间顺序检测失控并产生违规报告，
计算过程能力指数（Cp/Cpk/Pp/Ppk/Cpm/CPU/CPL、DPMO、合格率、sigma 等级），
不满足前置条件时返回 not_estimable 状态，支持排除/恢复测量点后按剩余
基线重算，进程重启后从数据库测量点完全重算控制限、能力指数与失控违规
集合并做一致性校验。状态持久化到 SQLite，进程重启后逐位恢复。

## 解决的问题

制造业产线或检测实验室对一个连续过程的质量特性做统计过程控制：操作员
按时间顺序抽检，把测量值录入系统，系统必须随时算出控制限、判断过程是否
处于统计控制状态、给出能力评估。手工计算在规则多、测量点多、中途排除
异常点时极易出错，且无法保证“控制限与违规是从测量点完全重算的纯函数”
这一不变量。spc 把控制图、Westgard 多规则检测、能力指数、排除重算、重启
恢复和一致性校验都做成服务，所有限值与违规都从数据库测量点重算得到，
不单独缓存权威值，因此重启后逐位相等。

## 主要输入 / 输出

- 输入：控制图（名称、质量特性名、图类型、单位、子组大小 n、USL/LSL/Target
  规格限）、测量点（单值 / 子组 n 值 / 不合格品数 d 与样本量 n）、排除/恢复
  操作、Westgard 规则开关。
- 输出：控制图详情（含当前控制限、组内/总体标准差、基线点数）、测量点列表
  （含 ±1σ/±2σ/±3σ 分区）、失控违规列表（规则名、严重等级、触发子组序号）、
  能力报告（各指数、DPMO、合格率、sigma 等级、是否可估计）、规则开关配置、
  全局统计、一致性校验报告、前端页面。

## 本地命令

```bash
go build ./...          # 编译
go run .                # 启动 HTTP 服务（默认 :8080，SQLite spc.db）
go run . --smoke-test   # 自检（不依赖外部服务、不睡眠）
go test ./...           # 单元/集成测试
```

环境变量 `SPC_ADMIN_TOKEN` 覆盖管理端点（POST /admin/recompute）的共享
密钥，默认 `admin-secret`。`--db <path>` 指定 SQLite 文件，默认 `spc.db`。
前端页面（原生 HTML/CSS/JS，无构建步骤）通过 `embed.FS` 内嵌进二进制，
启动后访问 `http://localhost:8080/`。

## HTTP API（主要，23 条路由）

- `GET /healthz` · `GET /version`
- `POST /charts` · `GET /charts` · `GET /charts/{id}` · `PATCH /charts/{id}` · `DELETE /charts/{id}`
- `POST /charts/{id}/measurements` · `GET /charts/{id}/measurements` · `GET /charts/{id}/measurements/{mid}` · `DELETE /charts/{id}/measurements/{mid}` · `POST /charts/{id}/measurements/{mid}/restore`
- `GET /charts/{id}/limits` · `GET /charts/{id}/zones` · `GET /charts/{id}/violations`
- `GET /charts/{id}/capability`
- `GET /charts/{id}/rules` · `PUT /charts/{id}/rules`
- `POST /charts/{id}/recompute`
- `GET /stats` · `POST /admin/recompute`
- `GET /` · `GET /static/{file}`（前端页面）

错误映射：参数错误 400、不存在 404、不变量/边界违反 422、状态冲突 409、
admin 鉴权失败 401。

## 前端

前端为原生 HTML/CSS/JS（无 Node、无构建步骤），源码位于 `internal/webfs/web/`
（通过 `//go:embed` 内嵌进二进制）。覆盖一个完整读写流程：建控制图 →
加测量点（按图类型）→ 看控制图（SVG 折线 + CL/UCL/LCL 引导线）→
查失控违规 → 查过程能力 → 排除点重算。前端依赖安装/构建命令：无（原生
静态页面，随 Go 服务在容器内可用）。

一条同时验证页面与业务 API 的 smoke-test 命令：

```bash
go run . --smoke-test
# 或在 benzhi 镜像内：
docker run --rm go-task-benzhi:amd64 sh -c 'cd /app && go build -o /tmp/spc . && /tmp/spc --smoke-test'
```

该 smoke-test 会请求 `GET /`（前端页面）与业务 API（测量点/限值/违规/
能力/规则/一致性校验）并校验。

## Docker

构建脚本的两个参数：镜像名、平台。

```bash
bash ./build_benzhi_docker.sh go-task-benzhi:amd64 linux/amd64   # amd64
bash ./build_benzhi_docker.sh go-task-benzhi:arm64 linux/arm64   # arm64
docker run -it go-task-benzhi:amd64        # 进入容器
```

多架构镜像（本机 Go 1.26.3 工具链，CGO_ENABLED=0）：

```bash
docker buildx build --platform linux/amd64 --load -t task120-spc:amd64 -f Dockerfile .
docker buildx build --platform linux/arm64 --load -t task120-spc:arm64 -f Dockerfile .
docker run --rm task120-spc:amd64 --smoke-test
docker run --rm task120-spc:arm64 --smoke-test
```
