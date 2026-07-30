# Formatter 文档导航

> 生成时间：2026-08-01 ｜ 代码版本：master@9b25e57

## 目录结构

```
docs/
├── README.md                      # 本文档 — 文档导航
├── onboarding.md                  # 新人上手指南
├── guide.md                       # 开发维护手册
└── architecture/                  # 架构文档 (C4 模型 + 设计决策)
    ├── overview.md                #   项目总览 (C1 + 技术栈 + 设计决策与权衡)
    ├── tech-architecture.md       #   技术架构 (C2 容器 + C3 组件)
    └── business-architecture.md   #   业务架构 (语言矩阵 + 包职责 + 流程)
```

## 文档索引

### 架构文档 (`architecture/`)

C4 模型分层描述系统架构，从系统上下文到组件细节。

| 文档 | 说明 | 适用读者 |
|------|------|----------|
| [overview.md](architecture/overview.md) | 项目总览 — C1 系统上下文 + 技术栈 + 设计决策与权衡 | 所有人 |
| [tech-architecture.md](architecture/tech-architecture.md) | 技术架构 — C2 容器视图 + C3 组件视图 + 横切关注点 | 架构师 / 开发者 |
| [business-architecture.md](architecture/business-architecture.md) | 业务架构 — 语言矩阵 + 包职责 + 核心业务流程 | 开发者 |

### 开发者指南

| 文档 | 说明 | 适用读者 |
|------|------|----------|
| [onboarding.md](onboarding.md) | 新人上手 — 环境搭建 + 启动 + 第一个任务 | 新成员 |
| [guide.md](guide.md) | 开发维护手册 — 规范 + 设计模式 + 新增功能 + 测试 + 构建 + 硬约束 | 开发者 |

## 推荐阅读顺序

1. **新成员**: overview → onboarding → guide
2. **架构师**: overview → tech-architecture → business-architecture
3. **开发者**: overview → business-architecture → guide
