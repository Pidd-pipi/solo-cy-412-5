# SmartEstate 智慧社区物业管理系统

> 面向业主、物业人员和管理员的数字化社区服务平台：在线报修、模拟缴费、公告发布、个人房产资料和物业工作台一体化管理。

## Docker 一键启动（推荐）

```bash
docker compose up -d
```

启动完成后访问：

- 前端：<http://localhost:18412>
- 后端健康检查：<http://localhost:19412/healthz>
- 后端 API：<http://localhost:19412/api/v1>

演示帐号密码均为 `password123`：业主 `13800000001`、物业 `13800000002`、管理员 `13800000003`。

## 主要功能

- **物业工作台**：汇总待办报修、本月已收费用和近期公告，并展示**待巡检、停用设施、未闭环巡检工单**数量。
- **公共设施巡检与停用处置**：物业按设施与巡检周期（日/周/月/季/年）建立计划，到期自动/手动幂等生成巡检任务；同一设施同一周期只能有一项计划、同一期次只能有一项任务，计划重跑不产生重复任务。巡检发现安全隐患时设施**立即停用并只生成一张关联维修工单**；维修完成后安排复检，复检未通过保持停用并续建维修单，复检通过才恢复可用。并发接单、重复提交均只有一个结果，终态记录不可改写。
- **报修管理**：业主创建水电/家具/公共设施等报修；物业筛选、分配和更新进度。巡检关联工单在原有报修流程中一并可处理。
- **费用缴纳**：按业主展示账单，通过支付宝沙箱模拟完成支付和记录查询。
- **社区公告**：置顶、发布、详情查看与阅读计数。
- **个人中心**：更新昵称、头像 URL，并绑定楼栋、单元和房间。
- **安全与治理**：JWT 登录态、RBAC、操作日志、敏感接口内存限流、统一 JSON 响应。

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus + ECharts |
| 后端 | **Go 1.22 + Gin + GORM** |
| 数据库 | MySQL 8.0（本地开发未设置 DSN 时回退 SQLite） |
| 认证 | JWT + RBAC |
| 部署 | Docker Compose + Nginx 反向代理 |

## 本地开发（备选）

前端：

```bash
cd frontend
npm install
npm run dev
```

后端：

```bash
cd backend && go mod tidy && go run ./cmd/server
```

构建后端：

```bash
cd backend && go build ./...
```

默认本地后端采用 SQLite 文件 `backend/smartestate.db`。若要连接 MySQL，请设置 `DB_DRIVER=mysql` 和 `DB_DSN`。

## 常用 API 清单

所有业务接口均以 `/api/v1` 开头，并使用 `{ "code": 0, "message": "ok", "data": ... }` 响应包裹。除登录与健康检查外均需 `Authorization: Bearer <token>`。

| 方法 | 接口 | 用途 / 权限 |
| --- | --- | --- |
| POST | `/auth/login` | 登录（限流） |
| GET/PUT | `/users/me` | 获取或更新个人资料 |
| GET | `/users/staff` | 获取处理人员，`repair:manage` |
| GET/POST | `/repairs` | 工单列表 / 创建工单 |
| PATCH | `/repairs/:id/assign` | 分配处理人，`repair:manage` |
| PATCH | `/repairs/:id/status` | 更新进度，`repair:manage` |
| GET/POST | `/payments` | 账单列表 / 生成账单 |
| POST | `/payments/:id/pay` | 模拟支付（限流） |
| GET/POST | `/announcements` | 公告列表 / 发布，发布需 `announcement:publish` |
| GET | `/announcements/:id` | 公告详情并记录阅读 |
| GET | `/dashboard/summary` | 工作台汇总（含巡检三项指标） |
| GET/POST | `/facilities` | 设施列表（`?status`）/ 新增设施，`inspection:manage` |
| GET | `/facilities/:id` | 设施详情：状态 + 巡检任务 + 关联维修工单进度 |
| GET/POST | `/inspection-plans` | 巡检计划列表 / 按设施+周期建计划（重复返回 409） |
| POST | `/inspection-plans/generate` | 到期生成任务，重跑幂等不重复 |
| GET | `/inspection-tasks` | 任务列表（`?status&kind&facility_id&open=true`） |
| PATCH | `/inspection-tasks/:id/claim` | 接单，并发仅一人成功（其余 409） |
| POST | `/inspection-tasks/:id/routine` | 提交常规巡检 `normal|hazard`；hazard 停用设施并生成唯一工单 |
| POST | `/inspection-tasks/:id/recheck` | 提交复检 `pass|fail`；仅当全部隐患链闭环时 pass 恢复，否则标记 `recheck_passed` 并保持停用；fail 续建工单 |
| GET | `/operation-logs` | 操作日志，`log:read` |

OpenAPI 摘要位于 `backend/api/openapi.yaml`。

## 项目结构

```text
.
├── frontend/
│   ├── src/api/                # user、repair、payment、announcement、facility、inspectionPlan、inspectionTask 请求
│   ├── src/stores/             # authStore、userStore、repairStore、paymentStore
│   ├── src/types/              # 共享实体和 permission 类型
│   ├── src/components/common/  # StatCard、RepairStatusBadge、RepairCard、FacilityCard、InspectionTaskCard 等
│   ├── src/hooks/              # useAuth、useRepairStats、usePermission
│   ├── src/pages/              # Dashboard、Repairs、Payments、Announcements、Facilities、Inspections、Profile
│   ├── src/router/             # 路由及 guards
│   ├── src/utils/              # request、roleText、feeCalculator
│   └── src/constants/          # repair、user、errorCodes
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/{config,model,repository,service,handler,router,middleware,dto,constants,util}
│   │   - 巡检模块：model/{facility,inspection_plan,inspection_task}.go、
│   │     repository/{facility,inspection_plan,inspection_task}_repository.go、
│   │     service/{facility,inspection_plan,inspection_task}_service.go + inspection_flow.go/inspection_cycle.go、
│   │     handler/{facility,inspection_plan,inspection_task}_handler.go、router/{facilities,inspection_plans,inspection_tasks}.go
│   ├── migrations/
│   ├── api/openapi.yaml
│   └── Dockerfile
├── database/init.sql
├── docker-compose.yml
└── .env.example
```

## 贯穿全栈的实体与分层

`User` 依次存在于数据库/GORM 模型、`model/user.go`、`repository/user_repository.go`、`service/user_service.go`、`handler/user_handler.go`、`router/users.go`、前端 `api/user.ts`、`stores/userStore.ts` 与共享类型。`Repair`、`Payment`、`Announcement` 均按模型→仓储→服务→处理器→路由→前端 API→页面/组件分层，CRUD 没有合并在单一文件。

**严禁合并职责到单一文件。** 当前实现按 `handler → service → repository → model` 单向依赖拆分。为了兼容原始练习的“牵一发动全身”约束，日志模板、权限码、枚举、格式化与提示文案也分散在指定常量/工具/路由/前端文件中；这是题目规定的高耦合演示设计，不应作为新生产系统的推荐模式。

## 枚举出现位置清单

### RepairStatus

- 后端定义：`backend/internal/constants/repair.go`；数据库 `Repair.status`；模型 `backend/internal/model/repair.go`。
- 后端使用：`backend/internal/service/repair_service.go` 状态机、`backend/internal/handler/repair_handler.go` DTO 校验、`backend/internal/constants/log_templates.go`、`backend/internal/util/formatter.go`。
- 前端定义：`frontend/src/constants/repair.ts`、`frontend/src/types/index.ts`。
- 前端使用：`frontend/src/components/common/RepairStatusBadge.vue`、`RepairCard.vue`、`frontend/src/pages/Repairs.vue` 的筛选器、`frontend/src/api/repair.ts`、`frontend/src/hooks/useRepairStats.ts`。

### UserRole

- 后端定义：`backend/internal/constants/user.go`；数据库 `User.role`；模型 `backend/internal/model/user.go`。
- 后端使用：`backend/internal/service/permission_service.go`、`backend/internal/middleware/auth.go`、`middleware/rbac.go`、路由权限与 `backend/internal/util/formatter.go`。
- 前端定义：`frontend/src/constants/user.ts`、`frontend/src/types/index.ts`。
- 前端使用：`frontend/src/stores/authStore.ts`、`frontend/src/hooks/useAuth.ts`、`usePermission.ts`、`frontend/src/router/index.ts` 的 meta、`router/guards.ts`、`components/common/PermissionButton.ts`、`utils/roleText.ts` 与 `App.vue`。

### FacilityStatus / InspectionTaskStatus / InspectionCycle（巡检模块）

- 后端定义：`backend/internal/constants/inspection.go`；数据库列 `facilities.status`、`inspection_tasks.status/kind/cycle`；模型 `model/facility.go`、`model/inspection_plan.go`、`model/inspection_task.go`。
- 后端使用：`service/inspection_task_service.go` 状态机、`service/inspection_flow.go`（停用/恢复/复检/续建）、`service/repair_service.go`（维修完成安排复检）、`repository/inspection_task_repository.go` 条件更新、`util/formatter.go`、`dto/requests.go` 的 `oneof` 校验、`constants/log_templates.go`。
- 前端定义：`frontend/src/constants/inspection.ts`、`frontend/src/types/index.ts`。
- 前端使用：`components/common/FacilityStatusBadge.vue`、`InspectionTaskStatusBadge.vue`、`FacilityCard.vue`、`InspectionTaskCard.vue`、`pages/Facilities.vue`、`pages/Inspections.vue`、`pages/Dashboard.vue`、`api/facility.ts`、`api/inspectionPlan.ts`、`api/inspectionTask.ts`。

## 巡检模块的幂等与并发一致性

- **唯一约束兜底**：计划 `(facility_id, cycle)` 唯一；任务 `(facility_id, kind, cycle, period_value)` 复合唯一、复检任务 `source_repair_id` 唯一；维修单 `source_task_id` 唯一。计划重跑/到期重算最多生成一条任务、一张隐患工单、一次复检。
- **跨周期补齐**：服务跨过多个周期才重跑时，按计划开始日补齐“起始期次 → 当前期次”的全部到期任务，日/周(ISO)/月/季/年使用同一套日历边界（见 `service/inspection_cycle.go`），每个期次仅一条；已存在或已完成（终态）的期次只跳过、不改写。逐期次独立写入，中途失败时已成功期次保留，重试只继续缺失期次，待巡检数量实时 `COUNT` 不会重复累计。计划开始日期显式提供但非法（非 `YYYY-MM-DD`）或为零值时返回 400，绝不按当天静默处理。
- **条件更新（CAS）+ 数据库事务**：接单与结果提交使用 `WHERE id=? AND status IN (...)` 的条件更新；停用设施、生成工单、推进任务在同一事务内完成。多人同时接单或重复提交时只有一个请求影响 1 行，其余返回 `409`。
- **终态不可改写**：`done/hazard/recheck_failed/restored` 再提交一律拒绝（`409`），已完成记录不被后续调整覆盖。
- **计数实时化**：工作台“待巡检/停用设施/未闭环工单”均为实时 `COUNT`，不维护累加计数器，从根本上避免重复累计。
- **状态闭环**：隐患 → 设施停用 + 一张工单 → 维修完成 → 一次复检；复检未过保持停用并续建工单，循环至复检通过才恢复可用。
- **多隐患工单不提前恢复**：同一设施可同时存在多条独立隐患处置链（不同期次的隐患工单 + 各自复检）。一次复检通过只结束该复检任务（状态 `recheck_passed`），仅当该设施**所有**关联隐患维修单均已闭环（done/closed）且无待处理复检时，最后一条通过的复检才把设施置为 `restored` 并恢复可用；仍有待处理/处理中工单或待复检时一律保持停用。维修单也可不经复检直接关闭（`closed`）：若它是最后一条未闭环链，则在同一事务内直接完成最终闭环恢复。复检通过在事务内对设施行加锁，并发复检被串行化，只有最后一个闭环触发恢复。原业主报修流程（无 `facility_id`）行为保持不变。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `COMPOSE_PROJECT_NAME` | `smartestate` | Compose 项目及容器名前缀 |
| `DB_NAME` / `DB_USER` / `DB_PASSWORD` | `smartestate_db` / `smartestate_user` / `smartestate_pwd` | MySQL 应用数据库与账号 |
| `DB_ROOT_PASSWORD` | `smartestate_root` | MySQL root 密码 |
| `JWT_SECRET` | `change_me_to_a_long_random_string` | JWT 签名密钥，生产必须替换 |
| `FRONTEND_PORT` / `BACKEND_PORT` / `DB_PORT` | `18412` / `19412` / `3306` | 对外端口 |

## Docker 部署说明

Nginx 提供 SPA 静态资源并将 `/api/` 代理至 Docker 内部的 `backend:8080`；浏览器前端只请求同源 `/api`。MySQL 使用命名卷 `smartestate_mysql_data` 持久化。数据库健康后才启动后端，后端通过 `/healthz` 健康后才启动前端。

常见问题：

1. 端口被占用时，修改 `.env` 中的 `FRONTEND_PORT`、`BACKEND_PORT` 或 `DB_PORT` 后重新执行 `docker compose up -d`。
2. 需重置演示数据时运行 `docker compose down -v`，这会删除 MySQL 持久化数据。
3. 中文路径可正常使用：Compose 的 build context 采用相对路径，未将宿主绝对路径传入容器。

## License

MIT
