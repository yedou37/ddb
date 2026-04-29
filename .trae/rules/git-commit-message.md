---
alwaysApply: true
scene: git_message
---

为当前项目生成 Git commit message 时，遵循以下规则：

1. 始终使用英文编写 commit message，禁止使用中文。
2. 优先使用简洁明确的一行提交标题，不要添加多余寒暄或解释。
3. 默认采用 `type: summary` 风格，例如：
   - `feat: add row-shard-group hierarchy scene`
   - `fix: slow down rebalance scene transitions`
   - `refactor: simplify scene timing and labels`
4. `type` 优先从以下类别中选择：`feat`、`fix`、`refactor`、`docs`、`chore`。
5. 标题使用动词开头，描述本次实际改动，避免空泛词语，例如 `update stuff`、`misc changes`。
6. 标题尽量控制在 72 个字符以内。
7. 除非用户明确要求，否则不要自动生成多行正文；默认只提供单行 commit message。
8. 如果一次改动包含多个点，优先概括最核心的用户可感知变化，而不是罗列所有细节。
