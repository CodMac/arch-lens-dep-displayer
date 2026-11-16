package db

import (
	"database/sql"
	"fmt"
)

// Schema 定义 DuckDB 全部建表语句（对齐 docs/schema.md）。
// 分层：① 原始层（ingest 写入，全量重建）→ ② 维度层（配置驱动）→ ③ 预计算层（对齐前端交互）。
// 原始表语言无关；层级/聚合表带 dimension_id 维度驱动。
const Schema = `
-- ========== ① 原始层（ingest 写入，全量重建） ==========
-- 主键 (qn, kind)：C++ 声明/定义共享 QN，仅 qn 不唯一（决策 9）。
CREATE TABLE IF NOT EXISTS elements (
    qn             VARCHAR NOT NULL,
    kind           VARCHAR NOT NULL,
    name           VARCHAR,
    path           VARCHAR,
    start_line     INTEGER,
    end_line       INTEGER,
    signature      VARCHAR,
    doc            VARCHAR,
    comment        VARCHAR,
    namespace_qn   VARCHAR,       -- L1 归属（package / namespace / module），JSONL NamespaceQN
    enclosing_qn   VARCHAR,       -- L2 归属（直接词法外层），JSONL EnclosingQN
    type_qn        VARCHAR,       -- 最外层具名类型（宿主类型），JSONL TypeQN
    is_source      BOOLEAN,
    is_sugar       BOOLEAN,       -- 编译器隐式生成（Lombok/默认构造器/enum.values…）
    is_impl_detail BOOLEAN,       -- 派生：源码语法结构（LAMBDA/ANON/MREF/BLOCK），见 §6.6
    extra          JSON,          -- Modifiers / Annotation / Mores
    PRIMARY KEY (qn, kind)
);

-- 端点带 kind：C++ IMPL_LINK 两端 QN 相同，只能靠 Kind 区分（决策 10）。
CREATE TABLE IF NOT EXISTS relations (
    source_qn   VARCHAR NOT NULL,
    source_kind VARCHAR NOT NULL,
    target_qn   VARCHAR NOT NULL,
    target_kind VARCHAR NOT NULL,
    type        VARCHAR,
    path        VARCHAR,
    start_line  INTEGER,
    end_line    INTEGER,
    raw_text    VARCHAR
);

-- ========== ② 维度层（配置驱动，role 解耦层级号） ==========
CREATE TABLE IF NOT EXISTS dimensions (
    id           INTEGER PRIMARY KEY,
    language     VARCHAR NOT NULL,
    qn_separator VARCHAR DEFAULT '.',
    -- method_edge 两端 kind 白名单（与 member 层不同：Java 6 类，不含 VARIABLE/ANONYMOUS_CLASS）
    method_kinds VARCHAR,
    -- 依赖关系类型白名单（逗号分隔）；结构关系（CONTAIN/IMPORT/IMPL_LINK）不参与依赖边统计
    relation_types VARCHAR,
    structural_relation_types VARCHAR,
    imported_at  TIMESTAMP DEFAULT current_timestamp,
    UNIQUE (language)
);

-- kinds = 本层接受的 ElementKind 白名单（逗号分隔）；role ∈ namespace|type|member。
CREATE TABLE IF NOT EXISTS dimension_levels (
    dimension_id INTEGER,
    level        INTEGER,
    role         VARCHAR NOT NULL,
    kinds        VARCHAR NOT NULL,
    UNIQUE (dimension_id, level),
    UNIQUE (dimension_id, role)
);

CREATE TABLE IF NOT EXISTS hierarchy_node (
    dimension_id INTEGER,
    qn           VARCHAR,
    level        INTEGER,
    role         VARCHAR,
    parent_qn    VARCHAR,
    depth        INTEGER,
    kind         VARCHAR,
    PRIMARY KEY (dimension_id, qn, kind)
);

CREATE TABLE IF NOT EXISTS element_node (
    element_qn   VARCHAR,
    element_kind VARCHAR,
    dimension_id INTEGER,
    node_qn      VARCHAR,
    level        INTEGER,
    PRIMARY KEY (dimension_id, element_qn, element_kind)
);

-- ========== ③ 预计算层（对齐前端交互） ==========
-- 注：VARCHAR[] 的读写受 duckdbthin 限制：读回是 "[CALL, USE]" 字符串，需在 repository 解析；
-- 写：bind 不支持切片参数 → 预计算一律用 SQL array_agg 生成，不经 Go 传数组。

-- 3.1 类型级边：宿主类型归一化后的去重边（L1 汇聚源 / L2 类型包级 BFS）
CREATE TABLE IF NOT EXISTS type_edge (
    dimension_id INTEGER,
    source_qn    VARCHAR,
    target_qn    VARCHAR,
    types        VARCHAR[],
    edge_count   INTEGER,          -- COUNT(*)，加权
    site_count   INTEGER,          -- 去重引用点数
    cross_pkg    BOOLEAN,
    PRIMARY KEY (dimension_id, source_qn, target_qn)
);

-- 3.2 调用点明细：方法对 → 文件/行号（L2 边详情 / L1 展开）
--     无 Location 的关系也要存（P13），主键隐含 NOT NULL → 写 path='' / line=0 哨兵。
CREATE TABLE IF NOT EXISTS call_site (
    dimension_id INTEGER,
    source_qn    VARCHAR,
    source_kind  VARCHAR,
    target_qn    VARCHAR,
    target_kind  VARCHAR,
    type         VARCHAR,
    path         VARCHAR NOT NULL,
    line         INTEGER NOT NULL,
    call_count   INTEGER,
    PRIMARY KEY (dimension_id, source_qn, source_kind,
                 target_qn, target_kind, type, path, line)
);

-- 3.4 类型级扇入扇出（与阈值无关的原始指标）
--     weighted = SUM(edge_count)；distinct sites = SUM(site_count)（阈值判定用）
CREATE TABLE IF NOT EXISTS type_fan (
    dimension_id   INTEGER,
    qn             VARCHAR,
    kind           VARCHAR,        -- 宿主类型 kind（type_qn→kind 实测 1:1）
    fan_in         INTEGER,
    fan_out        INTEGER,
    fan_in_types   INTEGER,
    fan_out_types  INTEGER,
    fan_in_sites   INTEGER,
    fan_out_sites  INTEGER,
    PRIMARY KEY (dimension_id, qn)
);

-- 3.5 违规口径（服务端唯一事实来源；前端调参 → PUT → 重算 → 落库）
CREATE TABLE IF NOT EXISTS violation_config (
    dimension_id         INTEGER PRIMARY KEY,
    fan_warn             INTEGER DEFAULT 50,
    fan_severe           INTEGER DEFAULT 150,
    cycle_relation_types VARCHAR[],
    merge_nested         BOOLEAN DEFAULT TRUE,
    dedupe_same_site     BOOLEAN DEFAULT TRUE,
    updated_at           TIMESTAMP DEFAULT current_timestamp
);

-- 3.6 阈值命中结果（按当前 violation_config 预计算）
CREATE TABLE IF NOT EXISTS violation_hit (
    dimension_id INTEGER,
    qn           VARCHAR,
    metric       VARCHAR,          -- fanin | fanout（同一类型可两行）
    severity     VARCHAR,          -- warn | severe
    value        INTEGER,          -- 命中时用的值（dedupe 时是 fan_*_sites）
    type_count   INTEGER,          -- 去重依赖类型数
    PRIMARY KEY (dimension_id, qn, metric)
);

-- 3.7 类型级环（宿主类型归一化 + 仅跨包）
CREATE TABLE IF NOT EXISTS cycle_group (
    dimension_id INTEGER,
    group_id     INTEGER,
    member_count INTEGER,
    pkg_count    INTEGER,
    PRIMARY KEY (dimension_id, group_id)
);
CREATE TABLE IF NOT EXISTS cycle_member (
    dimension_id INTEGER,
    group_id     INTEGER,
    type_qn      VARCHAR,
    namespace_qn VARCHAR,
    PRIMARY KEY (dimension_id, group_id, type_qn)
);
CREATE TABLE IF NOT EXISTS cycle_edge (
    dimension_id INTEGER,
    group_id     INTEGER,
    source_qn    VARCHAR,
    target_qn    VARCHAR,
    types        VARCHAR[],
    site_count   INTEGER,
    PRIMARY KEY (dimension_id, group_id, source_qn, target_qn)
);

-- 3.8 包级上卷（避免每次请求在 Go 里重算 pkgTree）
CREATE TABLE IF NOT EXISTS pkg_rollup (
    dimension_id           INTEGER,
    qn                     VARCHAR,
    subtree_type_count     INTEGER,
    subtree_cycle_groups   INTEGER,
    subtree_high_fanin     INTEGER,
    subtree_high_fanout    INTEGER,
    direct_type_count      INTEGER,
    PRIMARY KEY (dimension_id, qn)
);

-- 3.9 规模指标
CREATE TABLE IF NOT EXISTS metrics (
    dimension_id INTEGER,
    kind         VARCHAR,
    count        INTEGER,
    PRIMARY KEY (dimension_id, kind)
);

-- 3.10 节点标注/收藏（按维度隔离）
CREATE TABLE IF NOT EXISTS annotations (
    dimension_id INTEGER,
    qn           VARCHAR,
    starred      BOOLEAN DEFAULT FALSE,
    note         VARCHAR DEFAULT '',
    created_at   TIMESTAMP DEFAULT current_timestamp,
    updated_at   TIMESTAMP DEFAULT current_timestamp,
    PRIMARY KEY (dimension_id, qn)
);

-- 4.0 schema 版本（版本不符即删库重建，不做旧库迁移）
CREATE TABLE IF NOT EXISTS schema_meta (
    version INTEGER NOT NULL
);
`

// SchemaVersion 表结构版本。改动 Schema 时必须递增：Ensure 发现不符会删库重建。
const SchemaVersion = 1

// MethodEdgeView 方法级边 VIEW：由 call_site 聚合，两端 kind 均 ∈ dimensions.method_kinds。
// 白名单与 member 层不同（Java 6 类，不含 VARIABLE/ANONYMOUS_CLASS，决策 15）；
// 不用 CLASS 目标特例——analyzer 633c58f 后构造器 CALL 已指向真实 METHOD（决策 16）。
const MethodEdgeView = `
CREATE VIEW method_edge AS
SELECT cs.dimension_id,
       cs.source_qn,
       cs.target_qn,
       array_agg(DISTINCT cs.type) AS types,
       SUM(cs.call_count)          AS edge_count
FROM call_site cs
JOIN dimensions d ON d.id = cs.dimension_id
WHERE list_contains(string_split(COALESCE(d.method_kinds,''), ','), cs.source_kind)
  AND list_contains(string_split(COALESCE(d.method_kinds,''), ','), cs.target_kind)
GROUP BY 1,2,3
`

// Init 建表 + 写 schema 版本 + 建派生 VIEW（新库专用）。
func Init(c *sql.DB) error {
	if _, err := c.Exec(Schema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	if _, err := c.Exec(`DELETE FROM schema_meta`); err != nil {
		return fmt.Errorf("reset schema_meta: %w", err)
	}
	if _, err := c.Exec(`INSERT INTO schema_meta (version) VALUES (?)`, SchemaVersion); err != nil {
		return fmt.Errorf("write schema_meta: %w", err)
	}
	if _, err := c.Exec(MethodEdgeView); err != nil {
		return fmt.Errorf("create view method_edge: %w", err)
	}
	return nil
}

// schemaCurrent 判断库内 schema 版本是否与当前代码一致。
func schemaCurrent(c *sql.DB) bool {
	var v int
	if err := c.QueryRow(`SELECT version FROM schema_meta LIMIT 1`).Scan(&v); err != nil {
		return false
	}
	return v == SchemaVersion
}
