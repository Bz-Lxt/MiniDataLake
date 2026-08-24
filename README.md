# MiniDataLake

面向开发者的流式轻量级数据湖仓分析平台（Mini DuckDB + Web SQL Workspace）。

## 1. 如何启动

```bash
docker compose up --build -d
```

浏览器打开 `http://127.0.0.1:18420`。健康检查：`http://127.0.0.1:18420/api/v1/health`。

## 2. 使用说明

将 CSV / JSON / Parquet 拖入左侧目录（单文件默认 ≤128MB）。等待摄取 Job 完成后，目录树出现文件名、虚拟表名、列名与类型。在 Monaco 中编写 SQL，`⌘/Ctrl+Enter` 执行。结果区使用虚拟滚动加载分页数据。

**表格组件说明**：Prompt 写的是 React + Vxe-table。Vxe-table 仅支持 Vue，与 React 无法共存（官方 issue #1662）。已按冻结需求改用 TanStack Table + TanStack Virtual，虚拟滚动目标不变。

**解析路径**：CSV / JSON 为自研并发 Chunk 解析；Parquet 用 `parquet-go` 解码后转入自研列容器与 DICT/RLE 编码。查询执行器全自研，未嵌入 DuckDB。

## 3. 服务列表及 API 说明

| 地址 | 说明 |
|---|---|
| http://127.0.0.1:18420/ | Web SQL Workspace |
| http://127.0.0.1:18420/api/v1/health | 健康检查 |
| http://127.0.0.1:18420/api/v1/catalog | 湖仓目录 |
| http://127.0.0.1:18420/api/v1/query | 执行 SQL |
| http://127.0.0.1:18420/api/v1/system/stats | 内存与压缩率 |

完整契约见 `docs/API.md`。

## 4. 测试账号

默认无需登录。若设置环境变量 `MDL_API_TOKEN`，则除 `/health` 外的 `/api/v1/*` 需携带 `Authorization: Bearer <token>`。

## 5. 题目内容

实现一个允许上传几十兆 CSV/Parquet/JSON、Go 列式分析、Web 写 SQL 的极客工具。核心包括并发 Chunk 解析、手写过滤/聚合算子、字典/RLE 压缩、`.mdl` 落盘与虚拟滚动结果表。

## 6. 项目结构

```
backend/          Go 引擎与 HTTP
frontend-user/   React 工作台
frontend-admin/  SOP 占位（无独立后台）
frontend-mp/     SOP 占位（非小程序）
testdata/        样例文件
docs/            需求 / 架构 / API / QA / 审核
```

## 7. API 模拟与切换指南

本项目无外部 API 依赖，无 Mock 模式。所有查询结果、压缩率、内存占用与耗时均为自研引擎对真实上传数据的运行时实测值。`testdata/` 下的合成/样例文件只是测试夹具，不是替代执行逻辑的假数据。
