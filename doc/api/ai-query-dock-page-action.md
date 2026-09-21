# 查询页 AI 坞：接口需求与范围说明

## 1. 目标

查询页中的 AI 坞需要帮助用户生成并验证查询语句，然后请求当前浏览器页面把语句填入并执行。服务端只负责生成页面动作请求，不执行浏览器动作，也不保存页面执行结果。

本需求只覆盖两条消息接口及其完成该流程所必需的内部运行时和内置技能：

- `POST /api/fc-model/assistant/message/new`
- `POST /api/fc-model/assistant/message/detail`

部署在开源 n9e 时，接口前缀可由现有路由配置映射；接口字段和响应契约以本说明为准。

## 2. 新消息接口

### 2.1 请求体

`query` 需要支持以下字段：

```json
{
  "chat_id": "f994e541-c52b-4900-8b29-49f8c49d3de7",
  "query": {
    "content": "<@explorer-query> 帮我生成一个查询主机 CPU 使用率的语句",
    "references": [
      {
        "id": "explorer-query",
        "name": "explorer-query",
        "type": "skill",
        "skill": {
          "name": "explorer-query"
        }
      }
    ],
    "action": {},
    "page_from": {
      "url": "/metric/explorer",
      "param": {
        "datasource_type": "prometheus",
        "datasource_id": 849,
        "query": "",
        "start": "1789565434",
        "end": "1789569034"
      }
    },
    "page_actions": [
      {
        "name": "set_metric_query",
        "description": "Fill and run a PromQL expression in the current query panel. Send a statement verified against the selected data source. The page reports its actual query result. Nothing is saved.",
        "inputSchema": {
          "type": "object",
          "properties": {
            "promql": {
              "type": "string",
              "description": "The complete PromQL expression to run."
            }
          },
          "required": ["promql"]
        }
      }
    ]
  }
}
```

字段含义：

- `content`：用户问题，不能为空。
- `references`：用户或页面显式选择的资源。当前只消费 `type` 为 `skill` 的条目，并使用其中的技能名。未显式引用时不得根据页面或文本自动引用技能。
- `page_from.param`：当前查询页快照，例如数据源类型、数据源 ID、已有语句和时间范围。它必须传入本轮提示词和查询技能，使模型按当前数据源验证语句。
- `page_from.url`：页面地址，仅用于补充页面上下文。
- `page_actions`：当前页面已注册的浏览器动作清单。服务端不实现这些动作，也不根据固定动作表推断页面能力。
- `action`：沿用既有会话参数语义；本需求不扩展其含义。

### 2.2 页面动作声明的处理

服务端只接受可安全展示给模型的声明；无效条目应从本轮动作清单中排除，不能让整条普通对话请求失败。

限制如下：

- 最多 16 个动作。
- 动作名使用 ASCII 标识符：首字符为字母，后续字符为字母、数字、下划线或连字符，最长 64 字节。
- 描述不能为空，最多 500 个 Unicode 字符。
- 每个 `inputSchema` 最多 8 KiB。
- 顶层 schema 只允许 `type`、`properties`、`required`、`additionalProperties`；顶层类型只能是 `object`。
- 参数属性只允许 `type`、`description`、`enum`、`properties`、`required`、`items` 及其嵌套结构。参数类型只允许 `string`、`number`、`integer`、`boolean`、`array`、`object`。
- `additionalProperties` 仅允许在顶层，且必须是布尔值。
- `required` 必须为字符串数组，且其中每一项都必须在同一层的 `properties` 中声明。

服务端不校验模型给出的动作参数是否完全符合页面 schema。浏览器页面是动作实现者和参数校验者。

## 3. 内置技能

需要提供名为 `explorer-query` 的内置技能，并在服务启动时随其他内置技能一起可发现、可加载。

技能职责：

- 读取当前页面提供的数据源、已有查询和时间范围。
- 用只读查询工具验证候选 PromQL、SQL 或日志查询。
- 验证成功后，如果本轮提供了页面动作，调用页面动作请求浏览器填入或运行语句。
- 不声称浏览器已经执行动作，也不声称获得了浏览器执行结果。

显式引用 `explorer-query` 时，本轮必须加载该技能。该规则只为显式引用服务，不增加自动技能选择逻辑，也不改变其他业务动作的必备技能优先级。

## 4. 模型与页面动作流程

1. 服务端把当前页面已声明的动作名称、描述和受限 schema 写入本轮模型上下文。
2. 服务端同时提供固定工具 `page_action(name, args)`。
3. `name` 只能是本轮 `page_actions` 中声明的动作名；`args` 是该动作的参数对象。
4. 模型选择一个页面动作后，本轮结束。服务端不执行该动作，不把它当作可重放的服务端工具调用。
5. 服务端将模型选择的 `call_id`、动作名、描述和参数保存为本轮结构化响应。
6. 浏览器从详情响应读取该结构化响应，按本页已注册的动作执行。
7. 页面执行成功、失败或查询结果不会自动作为下一条用户消息提交给模型；如需继续对话，由用户或页面显式发起下一轮。

## 5. 详情接口响应

当模型调用页面动作后，`POST /api/fc-model/assistant/message/detail` 的 `response` 数组需要包含以下项：

```json
{
  "content": "Fill and run a PromQL expression in the current query panel. Send a statement verified against the selected data source. The page reports its actual query result. Nothing is saved.",
  "content_type": "page_action",
  "hint_text": "",
  "is_finish": true,
  "is_from_ai": true,
  "loop_id": "",
  "param": {
    "call_id": "b552162f-a424-4406-a355-c37d106d5e14",
    "name": "set_metric_query",
    "description": "Fill and run a PromQL expression in the current query panel. Send a statement verified against the selected data source. The page reports its actual query result. Nothing is saved.",
    "args": {
      "promql": "cpu_usage_active{cpu=\"cpu-total\"}"
    }
  },
  "stream_id": ""
}
```

`content_type` 为 `page_action` 时，`param` 是执行所需的唯一结构化载荷。`content` 仅供界面展示动作说明。

## 6. 验收

- 示例请求中的 `references`、`page_from.param` 和 `page_actions` 能进入本轮运行时。
- 显式引用 `explorer-query` 后，技能可以读取当前数据源和时间范围并调用只读查询工具。
- 模拟模型调用 `page_action` 后，详情响应包含完整的 `page_action` 结构。
- 未声明的动作、非法动作 schema 不会被模型选择。
- 历史消息不会重新执行已发出的页面动作。
- 查询页可以完成一次从语句生成、数据源验证到浏览器本地执行的回放。

## 7. 最小修改边界

以下内容是本需求必需的：

- 消息请求和详情响应的数据模型扩展。
- 新消息接口接收、筛选并传递显式技能引用、页面上下文和页面动作声明。
- 内部模型运行时将固定 `page_action` 工具暴露给模型，并把模型调用转换为详情响应。
- `explorer-query` 内置技能及与其相关的测试。
- 只覆盖上述流程的路由、模型和运行时测试。

以下内容不属于本需求：

- A2A 协议、A2A artifact 结构或 A2A 客户端兼容性改造。
- Redis stream 的既有 wire 协议扩展。
- 与查询页无关的页面动作支持。
- 创建、编辑、告警、仪表盘等业务流程的权限、表单和上下文规则改造。
- 自动选择技能、根据页面推断技能或改变其他 action 的技能优先级。

## 8. 当前变更范围审计

### 8.1 与需求直接相关的改动

- 请求和响应模型增加页面上下文地址、显式技能引用、页面动作声明及 `page_action` 响应类型。
- 页面动作声明校验、固定工具定义、模型调用解析和详情响应构造。
- 路由层将当前页面上下文、显式技能和页面动作传给本轮模型运行时。
- 新增 `explorer-query` 内置技能。
- 对上述行为的模型、路由和技能测试。

其中，模型运行时改动是必要的：页面动作不是服务端已有工具，必须通过运行时把模型的固定工具调用转换为详情响应中的结构化数据。

### 8.2 审计结论、问题成立条件与处理建议

以下问题在审计中被提出。每项都要根据实际调用链判断，不能因为代码层面存在改善空间就扩大本次协议。

1. **流式响应没有页面动作参数**

   如果页面动作被发送到 Redis stream，且消费者依赖 stream 执行动作，那么只有动作描述而没有 `call_id`、`name` 和 `args` 确实无法执行。

   本需求的浏览器不消费这条 stream 响应，而是在消息完成后调用详情接口；详情接口已经携带完整 `param`。本次也不把 `page_action` 发布到 Redis stream。因此这不是当前查询页流程的缺陷，stream wire 协议和 A2A artifact 改造均已撤回。未来若明确要求 A2A 或 stream 消费者执行页面动作，应单独定义该协议的完整载荷、版本兼容方式和消费者行为。

2. **页面上下文影响创建预检**

   这是实际问题。通用聊天上下文同时被查询技能和创建预检读取。若把任意页面的 `page_from.param` 无条件合并到该上下文，页面中的 `datasource_id`、`busi_group_id` 或 `team_ids` 可能被创建预检视为用户已选择的值，从而跳过原本应展示的选择步骤。

   当前最小实现仍将页面快照合并到通用聊天上下文，因此该风险尚未消除。应在后续单独决定处理方式：可以只在“查询页 + 普通对话 + 显式引用 `explorer-query`”这一条链路合并页面快照，也可以为查询技能增加与创建预检隔离的页面上下文。两种方案都需要补充创建流程回归测试；本次不在文档之外改动代码。

3. **显式技能引用覆盖全局技能优先级**

   为了加载 `explorer-query` 而修改所有 action 的技能优先级，会使带有业务必备技能的 action 意外改走浏览器引用的技能集合。这会产生与查询页无关的行为变化。

   本次保留局部加载：仅当查询页普通对话显式引用 `explorer-query` 时预加载它。其他 action 继续使用原有的必备技能、agent 绑定技能和兜底目录规则。

4. **非流式、同轮多个工具和失败展示**

   非流式调用无法接收页面动作、模型同轮调用多个工具可能造成历史记录取舍、失败动作没有对应展示帧等，都是运行时边界问题。但当前消息处理固定建立内部通道，查询技能通常先在前一轮完成只读验证，再在后续轮请求页面动作；本需求没有定义这些扩展场景的外部行为。

   因此非流式拒绝、兄弟工具 transcript 裁剪和 stream/A2A 展示调整均不保留在本次修改中。若未来需要支持，应先补充独立验收场景和接口契约。

5. **schema 支持范围**

   常规 JSON Schema 支持比本需求更多的关键字，例如嵌套 `additionalProperties`、`pattern`、`$defs` 和远程引用。查询页动作声明只允许受限子集，目的是让模型提示词稳定、可控，并避免将页面自定义 schema 原样扩大到模型工具上下文。

   因此，受限子集之外的合法 JSON Schema 被过滤不是缺陷；这是本次动作声明的明确契约。浏览器仍可在本地使用完整 schema 校验实际动作参数。

### 8.3 当前提交边界

本次修改只保留“请求入参 → 查询页显式技能和页面上下文 → 固定页面动作工具 → 详情响应”的最短链路。查询页显式引用 `explorer-query` 的加载分支只作用于查询页的普通对话路径，不改变其他 action 的技能优先级。页面上下文与创建预检的交叉风险见 8.2，尚待独立决策。
