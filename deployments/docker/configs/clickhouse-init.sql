-- ════════════════════════════════════════════════════════
--  ObserveX ClickHouse Schema
--  All persistent storage: services, topology, events, profiles
-- ════════════════════════════════════════════════════════

-- Services registry
CREATE TABLE IF NOT EXISTS services (
    id          String,
    name        String,
    display_name String,
    kind        LowCardinality(String),
    namespace   LowCardinality(String),
    cluster     LowCardinality(String),
    node        String,
    deployment  String,
    health_state LowCardinality(String),
    health_score Float32,
    tech_stack  Array(String),
    agent_id    String,
    discovered_at DateTime,
    last_seen_at  DateTime,
    PRIMARY KEY (id, last_seen_at)
) ENGINE = ReplacingMergeTree(last_seen_at)
ORDER BY (id)
TTL last_seen_at + INTERVAL 90 DAY;

-- Topology edges
CREATE TABLE IF NOT EXISTS topology_edges (
    id              String,
    source_id       String,
    target_id       String,
    protocol        LowCardinality(String),
    calls_per_min   Float32,
    avg_latency_ms  Float32,
    error_rate      Float32,
    bytes_per_sec   Float32,
    updated_at      DateTime,
    PRIMARY KEY (id, updated_at)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (id)
TTL updated_at + INTERVAL 90 DAY;

-- Native ObserveX metrics
CREATE TABLE IF NOT EXISTS metrics (
    name        LowCardinality(String),
    value       Float64,
    service_id  String,
    labels      String,
    timestamp   DateTime,
    received_at DateTime DEFAULT now()
) ENGINE = MergeTree()
ORDER BY (timestamp, name, service_id)
TTL timestamp + INTERVAL 90 DAY;

-- Problems (incidents)
CREATE TABLE IF NOT EXISTS problems (
    id          String,
    class       LowCardinality(String),
    severity    LowCardinality(String),
    title       String,
    detail      String,
    service_id  String,
    service_name String,
    namespace   LowCardinality(String),
    pod_name    String,
    node_name   String,
    deployment  String,
    confidence  Float32,
    status      LowCardinality(String),
    detected_at DateTime,
    resolved_at Nullable(DateTime),
    updated_at  DateTime
) ENGINE = MergeTree()
ORDER BY (detected_at, id)
TTL detected_at + INTERVAL 365 DAY;

-- Remediations (AI agent audit log)
CREATE TABLE IF NOT EXISTS remediations (
    id           String,
    problem_id   String,
    action       String,
    params       String,  -- JSON
    status       LowCardinality(String),
    detail       String,
    dry_run      UInt8,
    verify_result String,
    started_at   DateTime,
    finished_at  DateTime
) ENGINE = MergeTree()
ORDER BY (started_at, id)
TTL started_at + INTERVAL 365 DAY;

-- Notifications sent
CREATE TABLE IF NOT EXISTS notifications (
    id          String,
    problem_id  String,
    channel     LowCardinality(String),
    title       String,
    body        String,
    auto_fixed  UInt8,
    sent_at     DateTime,
    status      LowCardinality(String)
) ENGINE = MergeTree()
ORDER BY (sent_at)
TTL sent_at + INTERVAL 180 DAY;

-- Profiles
CREATE TABLE IF NOT EXISTS profiles (
    id           String DEFAULT toString(generateUUIDv4()),
    service_id   String,
    profile_type LowCardinality(String),
    start_time   DateTime,
    duration_sec UInt16,
    data         String  -- base64 pprof data
) ENGINE = MergeTree()
ORDER BY (start_time, service_id)
TTL start_time + INTERVAL 30 DAY;

-- Agent registry
CREATE TABLE IF NOT EXISTS agents (
    id            String,
    node_name     String,
    cluster_name  LowCardinality(String),
    version       String,
    ip_address    String,
    os            LowCardinality(String),
    arch          LowCardinality(String),
    kernel_version String,
    ebpf_enabled  UInt8,
    status        LowCardinality(String),
    last_seen     DateTime,
    registered_at DateTime
) ENGINE = ReplacingMergeTree(last_seen)
ORDER BY (id)
TTL registered_at + INTERVAL 365 DAY;

-- Dashboards
CREATE TABLE IF NOT EXISTS dashboards (
    id          String,
    name        String,
    description String,
    owner_id    String,
    tags        Array(String),
    widgets     String,  -- JSON
    variables   String,  -- JSON
    time_range  String,
    is_template UInt8,
    created_at  DateTime,
    updated_at  DateTime
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (id)
TTL created_at + INTERVAL 3650 DAY;

-- SLOs
CREATE TABLE IF NOT EXISTS slos (
    id          String,
    name        String,
    service_id  String,
    service_name String,
    kind        LowCardinality(String),
    target      Float64,
    window      LowCardinality(String),
    sli         Float64,
    budget_total Float64,
    budget_left Float64,
    burn_rate_1h Float64,
    status      LowCardinality(String),
    updated_at  DateTime
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (id);

-- Alert rules
CREATE TABLE IF NOT EXISTS alert_rules (
    id          String,
    name        String,
    service_id  String,
    namespace   LowCardinality(String),
    expr        String,
    threshold   Float64,
    operator    String,
    duration    String,
    severity    LowCardinality(String),
    message     String,
    silenced    UInt8,
    created_at  DateTime
) ENGINE = ReplacingMergeTree(created_at)
ORDER BY (id);
