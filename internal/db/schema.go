package db

// Schema 定义 DuckDB 全部建表语句（对齐 docs/middleware.md §4）。
// 原始表语言无关；层级/聚合表带 dimension_id 维度驱动。
const Schema = `
-- 4.1 原始表（导入即建，语言无关）
CREATE TABLE IF NOT EXISTS elements (
    qn          VARCHAR,
    kind        VARCHAR,
    name        VARCHAR,
    path        VARCHAR,
    is_source   BOOLEAN,
    is_sugar    BOOLEAN,
    is_external BOOLEAN,
    start_line  INTEGER
);

CREATE TABLE IF NOT EXISTS relations (
    source_qn   VARCHAR,
    target_qn   VARCHAR,
    type        VARCHAR,
    start_line  INTEGER,
    raw_text    VARCHAR
);

-- 4.2 层级表（维度驱动）
CREATE TABLE IF NOT EXISTS dimensions (
    id          INTEGER PRIMARY KEY,
    language    VARCHAR,
    name        VARCHAR,
    imported_at TIMESTAMP DEFAULT current_timestamp
);

CREATE TABLE IF NOT EXISTS dimension_levels (
    dimension_id  INTEGER,
    level         INTEGER,
    kind          VARCHAR,
    parent_of     VARCHAR
);

CREATE TABLE IF NOT EXISTS hierarchy_node (
    dimension_id  INTEGER,
    qn            VARCHAR,
    level         INTEGER,
    parent_qn     VARCHAR,
    kind          VARCHAR
);

CREATE TABLE IF NOT EXISTS element_node (
    element_qn    VARCHAR,
    dimension_id  INTEGER,
    node_qn       VARCHAR,
    level         INTEGER
);

-- 4.3 聚合表（预计算后写回）
CREATE TABLE IF NOT EXISTS level_edges (
    dimension_id  INTEGER,
    level         INTEGER,
    source_qn     VARCHAR,
    target_qn     VARCHAR,
    edge_count    INTEGER,
    types         VARCHAR
);

CREATE TABLE IF NOT EXISTS fan (
    dimension_id  INTEGER,
    qn            VARCHAR,
    fan_in        INTEGER,
    fan_out       INTEGER
);

CREATE TABLE IF NOT EXISTS scc (
    dimension_id  INTEGER,
    component_id  INTEGER,
    member_qn     VARCHAR
);

CREATE TABLE IF NOT EXISTS metrics (
    dimension_id  INTEGER,
    kind          VARCHAR,
    count         INTEGER
);

-- D6 需求14：节点标注/收藏（按维度隔离）
CREATE TABLE IF NOT EXISTS annotations (
    dimension_id  INTEGER,
    qn            VARCHAR,
    starred       BOOLEAN DEFAULT FALSE,
    note          VARCHAR DEFAULT '',
    created_at    TIMESTAMP DEFAULT current_timestamp,
    updated_at    TIMESTAMP DEFAULT current_timestamp,
    PRIMARY KEY (dimension_id, qn)
);
`
