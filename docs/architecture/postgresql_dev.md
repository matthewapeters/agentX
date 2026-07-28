# PostgreSQL Development Setup — AgentX

## Database Configuration

| Property | Value |
|----------|-------|
| Database | `agentx` |
| Dev User | `agentx_dev` |
| Password | `dev_password` |
| PostgreSQL Version | 18 |
| Extensions | `postgis`, `vector` (pgvector) |

**Connect**:
```bash
psql -U agentx_dev -d agentx -h localhost
```

---

## Migration Overview: JSON → PostgreSQL

AgentX currently uses file-based session storage (`working_memory.json`, in-memory conversation history persisted to disk). This document outlines the work to migrate to a PostgreSQL-based 4-D knowledge graph with PostGIS for time-windowed queries and pgvector for semantic similarity.

### Phase 1: Database Setup and Schema Design (1–2 days)

- Install PostgreSQL 18 with PostGIS 3.4 and pgvector extensions
- Design the core schema:
  - `graph_nodes` — conversations, facts, summaries, decisions (with embeddings)
  - `graph_edges` — relationships between nodes (mentioned, similar, leads_to) with weights
  - `sessions` — session metadata, current focus level, attachment points
- Create indexes:
  - GiST for spatial/time queries
  - ivfflat for vector similarity search
  - B-tree for time-range and ID lookups
- Configure connection pooling for the Go application

### Phase 2: Data Migration (2–3 days)

- Extract current file-based data (`working_memory.json`, conversation history, session metadata)
- Transform flat facts and linear history into graph model:
  - Each conversation turn → `graph_node` of type `conversation`
  - Each working memory fact → `graph_node` of type `fact`
  - Each session → `session` record
- Load transformed data into PostgreSQL
- Validate data integrity and consistency

### Phase 3: Session Layer Refactoring (5–7 days)

- Replace file I/O with database queries throughout the session layer
- Update `session.NewStore`, `session.New`, and related constructors
- Migrate all `WorkingMemory` operations:
  - `LoadWorkingMemory` → query `graph_nodes` filtered by session and type
  - `SaveWorkingMemory` → upsert nodes with appropriate edges
  - `SetFact`, `DeleteFact`, `SetFactEnabled` → node property updates
- Update session metadata persistence (ID, name, timestamps, focus level)
- Implement migration path for existing file-based sessions (one-time upgrade)

### Phase 4: Graph Construction (3–5 days)

- Implement graph node creation for all types (conversation, fact, summary, decision)
- Implement edge creation with typed relationships and weights
- Add embedding generation pipeline:
  - Call OpenAI or Ollama embedding API on node insert
  - Store 1536-dimension vectors in `graph_nodes.embedding`
- Build semantic edges automatically:
  - Find nodes with cosine similarity < 0.2
  - Create `SEMANTIC_SIMILAR` edges with weight = 1 - cosine_distance
- Implement graph traversal using recursive CTEs (BFS for context balloons)

### Phase 5: Query Layer (3–5 days)

- Implement lobby queries (low-focus, high-threshold filtering):
  - Filter: edge weight > 0.8, temporal window < 24h, focus_level < 5
  - Dimensions: temporal + semantic only
- Implement context balloon queries (multi-dimensional filtering):
  - Combine graph traversal, time windows, semantic similarity, edge weights
  - Weighted scoring: 0.5 semantic + 0.3 temporal + 0.2 graph
- Add focus level management:
  - Adjust filter thresholds dynamically based on session focus state
  - Low focus = coarse granularity (high thresholds)
  - High focus = fine granularity (low thresholds)
- Implement time-windowed queries (24h, 7d, 30d windows)
- Add semantic similarity search with vector distance filtering

### Phase 6: Testing and Validation (3–5 days)

- Unit tests for graph operations (node creation, edge traversal, embedding generation)
- Integration tests for session flow (create, load, update, focus transitions)
- Performance testing (query latency under various focus levels, embedding generation throughput)
- Behavioral validation: confirm new system produces equivalent results to file-based prior

---

## Rationale: Why PostgreSQL + PostGIS + pgvector?

### The Problem with Flat JSON Storage

AgentX's current working memory is a flat list of key-value facts persisted to `working_memory.json`. This approach has fundamental limitations:

1. **No relationship modeling**: Facts exist in isolation. There is no way to express that fact A was extracted from conversation B, or that conversation C is semantically similar to conversation D.
2. **No multi-dimensional querying**: You cannot efficiently query "recent conversations about authentication that are semantically similar to the current topic and have strong edges to the session seed."
3. **No focus management**: The context window is a rectangle — everything is loaded or nothing is. There is no mechanism for dynamic granularity based on session focus.
4. **No semantic dimension**: Facts and conversations are matched by exact key or linear search, not by meaning.

### The 4-D Knowledge Graph Solution

The migration targets a **4-D knowledge graph** where knowledge exists at multiple scales and is accessible through multi-dimensional queries:

| Dimension | What It Captures | Query Mechanism |
|-----------|-----------------|-----------------|
| **Spatial** | Semantic proximity between concepts | Vector cosine distance (pgvector) |
| **Temporal** | When events occurred, recency | Time-range filters (PostGIS `tsrange`) |
| **Semantic** | Meaning-based relationships | Edge weights + similarity thresholds |
| **Focus** | Session-level attention granularity | Dynamic filter thresholds |

### Strongest Arguments for This Approach

**1. Memory as a network, not a list**

Human memory is relational — facts are attached to conversations, conversations lead to new facts, facts inform future actions. A flat JSON list cannot express this topology. A graph can.

**2. Context window is a cube, not a rectangle**

The limit of an agent's memory is not context size but **focus capacity**. With a graph, you can load a shaped subgraph (context balloon) tailored to the current session focus, rather than dumping everything into the prompt. Low focus = coarse balloon (only strong edges, high-level summaries). High focus = fine balloon (weak edges, detailed conversations).

**3. Multi-dimensional filtering is native to PostgreSQL**

PostGIS provides time-windowed spatial queries out of the box. pgvector provides vector similarity search. Together, they enable queries like:

```sql
SELECT n.*
FROM graph_nodes n
JOIN graph_edges e ON n.id = e.to_id
WHERE e.from_id = :seed
  AND e.weight > 0.5
  AND n.embedding <=> :seed_embedding < 0.3
  AND n.timestamp > NOW() - INTERVAL '7 days'
ORDER BY 0.6 * semantic_score + 0.4 * temporal_score
LIMIT 50;
```

This is a true 4-D balloon query — combining graph traversal, semantic similarity, time windows, and weighted ranking in a single statement.

**4. PostGIS experience is directly transferable**

The geospatial balloon query pattern (find objects within N km and M minutes) maps directly to the context balloon problem (find nodes within N hops, cosine distance < D, and T time units). The indexing, query optimization, and tooling are the same.

**5. pgvector integrates natively with PostgreSQL**

No separate vector database, no sync overhead. Vector similarity search runs alongside SQL queries in the same transactional context. The `ivfflat` index provides fast nearest-neighbor search without the complexity of a dedicated vector store.

**6. ACID compliance for graph operations**

Graph construction (node/edge creation, semantic edge generation) must be transactional. PostgreSQL provides this natively. Dedicated graph databases (Neo4j, TigerGraph) have ACID, but PostgreSQL's maturity and ecosystem are superior for a production system.

**7. Session initiation as graph attachment**

Each new conversation is an opportunity to attach a new node to the knowledge map. The lobby (low-focus entry point) shows ranked past engagements as navigation indices. The user's response introduces a new node, and linking it to existing memory determines what context is relevant — the "context balloon" for the session. This is a graph operation, not a file I/O operation.

---

## Alternatives Considered

| Approach | Why Rejected |
|----------|-------------|
| **Flat JSON (status quo)** | Cannot express relationships, no multi-dimensional queries, no focus management |
| **Neo4j (native graph DB)** | Weaker time-windowed queries, separate system to maintain, no PostGIS integration |
| **RAG with vector DB** | Only handles semantic dimension, no graph traversal, no temporal filtering |
| **MemGPT-style explicit memory** | Requires agent to learn memory management pattern, adds prompt complexity |
| **ArangoDB (multi-model)** | Smaller community, less battle-tested, still requires separate system |

---

---

## Schema Design

### Overview

The schema models a 4-D knowledge graph where each node represents a discrete piece of knowledge (conversation turn, fact, summary, decision) and edges represent typed relationships between nodes. The `focus_level` column on nodes and `weight` column on edges enable dynamic granularity filtering — the core mechanism for the context balloon.

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────┐
│  sessions    │────>│   graph_nodes    │<────│  graph_edges │
│              │     │                  │     │              │
│ id (PK)      │     │ id (PK)          │     │ from_id (FK) │
│ name         │     │ session_id (FK)  │────>│ to_id (FK)   │
│ user_id      │     │ type             │     │ edge_type    │
│ created_at   │     │ content (JSONB)  │     │ weight       │
│ focus_level  │     │ embedding        │     │ created_at   │
│ current_seed │     │ focus_level      │     │              │
│ lobby_hash   │     │ timestamp        │     │              │
└──────────────┘     └──────────────────┘     └──────────────┘
```

### sessions

Tracks active and past sessions. Each session has a focus level (0–10) that controls how aggressively the context balloon filters edges and nodes. The `current_seed` is the graph node that anchors the current context balloon.

```sql
CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL DEFAULT '',
    user_id     TEXT NOT NULL DEFAULT '',
    -- Focus level: 0 = no focus (coarsest, lobby view), 10 = full focus (finest)
    -- Controls edge weight thresholds and node focus_level filters in queries.
    focus_level INT NOT NULL DEFAULT 0 CHECK (focus_level BETWEEN 0 AND 10),
    -- The seed node anchoring the current context balloon.
    -- When NULL, the lobby view is used (ranked past engagements).
    current_seed UUID REFERENCES graph_nodes(id) ON SET NULL,
    -- Hash of the lobby state for caching/invalidation.
    lobby_hash  TEXT,
    -- Session lifecycle
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at   TIMESTAMPTZ
);

-- Index: find active sessions for a user, ordered by recency
CREATE INDEX idx_sessions_user_active ON sessions (user_id, created_at DESC)
    WHERE closed_at IS NULL;

-- Index: find session by seed node (for context balloon resolution)
CREATE INDEX idx_sessions_seed ON sessions (current_seed)
    WHERE current_seed IS NOT NULL;

COMMENT ON COLUMN sessions.focus_level IS
    'Granularity of context: 0=lobby (high filter), 10=full work (low filter). ' ||
    'Used as a multiplier on edge weight thresholds and node focus_level filters.';

COMMENT ON COLUMN sessions.current_seed IS
    'Node ID anchoring the current context balloon. NULL means lobby view.';
```

### graph_nodes

The central table. Each row is a discrete piece of knowledge with a type, optional embedding, and focus metadata.

```sql
CREATE TABLE graph_nodes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Which session this node belongs to.
    -- Facts and summaries may be session-independent (user_id set, session_id NULL).
    session_id  UUID REFERENCES sessions(id) ON DELETE CASCADE,
    -- Type categorizes the node and determines which edges/queries apply.
    type        TEXT NOT NULL CHECK (type IN (
        'conversation',  -- A user prompt + assistant response turn
        'fact',          -- Extracted working memory fact
        'summary',       -- Summarized conversation or initiative
        'decision',      -- A recorded decision or commitment
        'plan',          -- A decomposition plan or task list
        'edge'           -- Internal: marks an edge node (for graph-only queries)
    )),
    -- Structured content. The shape depends on type:
    --   conversation: {user_text, assistant_text, role, ordinal}
    --   fact:         {key, value, source (user|agent|system)}
    --   summary:      {text, source_node_ids, scope}
    --   decision:     {text, rationale, source_node_ids}
    --   plan:         {goal, nodes[], status}
    content     JSONB NOT NULL DEFAULT '{}',
    -- 1536-dim vector for semantic similarity search (OpenAI text-embedding-3-small).
    -- NULL until embedding is generated. Indexed with ivfflat for fast nearest-neighbor.
    embedding   vector(1536),
    -- Focus level of this node: how "detailed" it is.
    -- Low focus (lobby) shows only nodes with focus_level <= 3.
    -- High focus (active work) shows nodes with focus_level <= 10.
    -- This is the node-side of the focus filter, complementary to edge weight filters.
    focus_level INT NOT NULL DEFAULT 5 CHECK (focus_level BETWEEN 0 AND 10),
    -- Timestamps for temporal dimension queries.
    -- timestamp: when the event occurred (conversation time, fact creation time)
    -- updated_at: when the node was last modified
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index: temporal queries (last N days/hours)
CREATE INDEX idx_nodes_timestamp ON graph_nodes (timestamp DESC);

-- Index: semantic similarity search (pgvector ivfflat)
-- lists=100 provides ~95% recall at 10x query speed vs brute force.
CREATE INDEX idx_nodes_embedding ON graph_nodes
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Index: node type for filtered queries
CREATE INDEX idx_nodes_type ON graph_nodes (type);

-- Index: session + type for session-scoped lookups
CREATE INDEX idx_nodes_session_type ON graph_nodes (session_id, type);

-- Index: focus_level for lobby filtering
CREATE INDEX idx_nodes_focus ON graph_nodes (focus_level) WHERE focus_level <= 5;

COMMENT ON COLUMN graph_nodes.type IS
    'Categorizes the node. Determines content schema and applicable edges.';

COMMENT ON COLUMN graph_nodes.embedding IS
    'Semantic vector. NULL until generated. Used for cosine similarity search.';

COMMENT ON COLUMN graph_nodes.focus_level IS
    'Node granularity: 0=high-level summary, 10=detailed conversation. ' ||
    'Lobby filters to focus_level<=3; active work shows all.';
```

### graph_edges

Typed, weighted relationships between nodes. Edge type determines semantics; weight determines visibility at different focus levels.

```sql
CREATE TYPE edge_type_enum AS ENUM (
    -- Graph/topology edges (how nodes relate structurally)
    'parent',           -- Child node belongs to parent (plan->task, summary->conversation)
    'mentioned',        -- Node B was mentioned in Node A
    'extracted_from',   -- Fact was extracted from a conversation
    'similar_to',       -- Semantic similarity (weight = 1 - cosine_distance)
    'contradicts',      -- Node B contradicts Node A
    'builds_on',        -- Node B extends or refines Node A
    'depends_on',       -- Node B cannot proceed without Node A
    
    -- Temporal edges (time-based relationships)
    'before',           -- Node A occurred before Node B
    'after',            -- Node A occurred after Node B
    'during',           -- Node A occurred during Node B
    'follows',          -- Node A immediately follows Node B
    
    -- Semantic edges (meaning-based relationships)
    'is_a',             -- Node A is a type of Node B (fact categorization)
    'part_of',          -- Node A is a component of Node B
    'related_to',       -- General semantic association
    'about',            -- Node A is about the topic of Node B
    
    -- Focus/session edges
    'in_session',       -- Node belongs to a session
    'resumed_from',     -- Session resumed from a prior node
    'branched_from'     -- Session branched from a concurrent session
);

CREATE TABLE graph_edges (
    from_id     UUID NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    to_id       UUID NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    -- Edge type determines semantics and query behavior.
    -- Some types are directional (before/after), others are symmetric (similar_to).
    edge_type   edge_type_enum NOT NULL,
    -- Weight: 0.0 to 1.0. Higher = stronger association.
    -- Focus filter: edges with weight < (0.3 + focus_level * 0.07) are hidden.
    -- At focus_level=0: only edges >= 0.3 visible (lobby)
    -- At focus_level=10: only edges >= 1.0 visible (only exact matches)
    -- Note: The formula is inverted in practice — lower threshold at higher focus.
    weight      FLOAT NOT NULL DEFAULT 0.5 CHECK (weight BETWEEN 0.0 AND 1.0),
    -- When the edge was created
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Primary key is the (from, to, type) tuple — an edge has a specific direction,
    -- type, and target. Multiple edges between the same pair are allowed if types differ.
    PRIMARY KEY (from_id, to_id, edge_type)
);

-- Index: find all edges from a node (outgoing traversal)
CREATE INDEX idx_edges_from ON graph_edges (from_id, edge_type);

-- Index: find all edges to a node (incoming, for "what mentions this?")
CREATE INDEX idx_edges_to ON graph_edges (to_id, edge_type);

-- Index: weight-based filtering for focus-dependent queries
CREATE INDEX idx_edges_weight ON graph_edges (weight DESC);

-- Index: temporal edges for time-range queries
CREATE INDEX idx_edges_temporal ON graph_edges (from_id, edge_type)
    WHERE edge_type IN ('before', 'after', 'during', 'follows');

-- Index: semantic edges for similarity graph traversal
CREATE INDEX idx_edges_semantic ON graph_edges (edge_type)
    WHERE edge_type IN ('similar_to', 'related_to', 'about');

-- Constraint: prevent self-loops
ALTER TABLE graph_edges ADD CONSTRAINT no_self_loops
    CHECK (from_id <> to_id);

-- Constraint: prevent duplicate edges of the same type
ALTER TABLE graph_edges ADD CONSTRAINT unique_edge
    EXCLUDE USING gist (from_id WITH =, to_id WITH =, edge_type WITH =,
                         pg_catalog.tsrange(created_at, created_at) WITH &&);

COMMENT ON COLUMN graph_edges.edge_type IS
    'Semantics of the relationship. Determines which queries include this edge. ' ||
    'Directional types: before, after, follows. Symmetric: similar_to, related_to.';

COMMENT ON COLUMN graph_edges.weight IS
    'Association strength 0.0-1.0. Focus filter hides edges below threshold. ' ||
    'Lobby (focus=0) sees weight>=0.8. Active work (focus=10) sees weight>=0.3.';
```

### focus_config

Configures how focus level maps to filter thresholds. This decouples the focus scale (0–10) from the actual query parameters, allowing tuning without code changes.

```sql
CREATE TABLE focus_config (
    id              INT PRIMARY KEY DEFAULT 1,  -- Single row: the global config
    -- Edge weight threshold at each focus level.
    -- edge_threshold[f] = minimum edge weight visible at focus level f.
    -- Focus 0 (lobby): only very strong edges (>= 0.8) visible.
    -- Focus 10 (full): weaker edges (>= 0.3) become visible.
    edge_threshold  FLOAT[] NOT NULL DEFAULT
        ARRAY[0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5, 0.45, 0.4, 0.35, 0.3],
    -- Node focus_level ceiling at each focus level.
    -- focus_config.node_ceiling[f] = maximum node focus_level visible at session focus f.
    -- Lobby (focus=0): only high-level nodes (focus_level <= 3) visible.
    -- Active work (focus=10): all nodes visible.
    node_ceiling    INT[] NOT NULL DEFAULT ARRAY[3, 4, 4, 5, 5, 6, 6, 7, 8, 9, 10],
    -- Temporal window in hours at each focus level.
    -- Lobby: only last 24 hours. Active: last 168 hours (7 days).
    temporal_hours  INT[] NOT NULL DEFAULT ARRAY[24, 48, 72, 96, 120, 144, 168, 168, 168, 168, 168],
    -- Max nodes to return at each focus level.
    -- Lobby: small set (20). Active: larger set (100).
    max_results     INT[] NOT NULL DEFAULT ARRAY[20, 30, 40, 50, 60, 70, 80, 90, 100, 100, 100],
    -- Semantic similarity threshold (cosine distance) at each focus level.
    -- Lower distance = more similar. Lobby: only very similar (<=0.1). Active: moderately similar (<=0.5).
    semantic_dist   FLOAT[] NOT NULL DEFAULT
        ARRAY[0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.5, 0.5]
);

-- Ensure only one row exists
CREATE UNIQUE INDEX idx_focus_config_single ON focus_config (id)
    WHERE id = 1;

COMMENT ON TABLE focus_config IS
    'Maps focus level (0-10) to query filter thresholds. ' ||
    'Allows tuning without code changes. Arrays are 11 elements (focus 0-10).';
```

### session_lobby_cache

Caches the lobby view (low-focus entry point) per session. The lobby shows ranked past engagements, recent summaries, and available focus levels. Caching avoids re-computing this on every session load.

```sql
CREATE TABLE session_lobby_cache (
    session_id  UUID PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    -- JSONB payload of the lobby:
    -- {
    --   "past_engagements": [{id, summary, timestamp, weight}],
    --   "recent_summaries": [{id, text, timestamp}],
    --   "available_levels": ["new_initiative", "new_feature", "continuation", "branch"],
    --   "generated_at": "2025-01-15T10:30:00Z"
    -- }
    lobby_data  JSONB NOT NULL DEFAULT '{}',
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index: invalidate stale cache (older than 5 minutes)
CREATE INDEX idx_lobby_cache_stale ON session_lobby_cache (generated_at)
    WHERE now() - generated_at > interval '5 minutes';

COMMENT ON TABLE session_lobby_cache IS
    'Cached lobby view per session. Regenerated when focus changes or after 5 minutes. ' ||
    'Lobby shows: ranked past engagements, recent summaries, engagement options.';
```

### Triggers

Automated maintenance triggers for `updated_at` timestamps and semantic edge creation.

```sql
-- Trigger: auto-update updated_at on graph_nodes
CREATE OR REPLACE FUNCTION fn_nodes_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at IS DISTINCT FROM OLD.updated_at THEN
        NEW.updated_at = now();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_nodes_updated_at
    BEFORE UPDATE ON graph_nodes
    FOR EACH ROW
    EXECUTE FUNCTION fn_nodes_updated_at();

-- Trigger: auto-update updated_at on sessions
CREATE OR REPLACE FUNCTION fn_sessions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at IS DISTINCT FROM OLD.updated_at THEN
        NEW.updated_at = now();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sessions_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION fn_sessions_updated_at();

-- Trigger: generate embedding after insert (placeholder — implement with CALL to embedding service)
CREATE OR REPLACE FUNCTION fn_generate_embedding()
RETURNS TRIGGER AS $$
DECLARE
    emb vector(1536);
BEGIN
    -- Skip if embedding already exists or content is empty
    IF NEW.embedding IS NOT NULL OR jsonb_typeof(NEW.content) != 'object' THEN
        RETURN NEW;
    END IF;
    
    -- Extract text from content based on type
    -- conversation: concatenate user + assistant text
    -- fact: concatenate key + value
    -- summary/decision/plan: use text field
    DECLARE
        text_content TEXT;
    BEGIN
        CASE NEW.type
            WHEN 'conversation' THEN
                text_content := COALESCE(NEW.content->>'user_text', '') || ' ' ||
                                COALESCE(NEW.content->>'assistant_text', '');
            WHEN 'fact' THEN
                text_content := COALESCE(NEW.content->>'key', '') || ': ' ||
                                COALESCE(NEW.content->>'value', '');
            WHEN 'summary' OR 'decision' OR 'plan' THEN
                text_content := COALESCE(NEW.content->>'text', '');
            ELSE
                RETURN NEW;
        END CASE;
        
        IF length(trim(text_content)) > 0 THEN
            -- TODO: Replace with actual embedding API call
            -- emb := openai_embedding(text_content);
            -- For now, set a placeholder zero vector
            emb := vector_repeat(0, 1536);
            NEW.embedding = emb;
        END IF;
    END;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_generate_embedding
    AFTER INSERT ON graph_nodes
    FOR EACH ROW
    EXECUTE FUNCTION fn_generate_embedding();
```

---

## Key Query Patterns

### Lobby Query (focus_level = 0)

Returns ranked past engagements and recent summaries. High filter: strong edges only, recent time window, high-level nodes only.

```sql
-- Step 1: Get the session's focus config
SELECT 
    edge_threshold[1] AS min_edge_weight,
    node_ceiling[1]   AS max_node_focus,
    temporal_hours[1] AS max_hours,
    max_results[1]    AS limit_rows,
    semantic_dist[1]  AS max_semantic_dist
FROM focus_config;

-- Step 2: Query with lobby thresholds
-- edge_threshold[1] = 0.8, node_ceiling[1] = 3, temporal_hours[1] = 24
WITH lobby_nodes AS (
    SELECT
        n.id,
        n.type,
        n.content,
        n.timestamp,
        n.focus_level,
        -- Rank by a combination of recency and edge strength
        0.5 * (1.0 / (EXTRACT(EPOCH FROM NOW() - n.timestamp) / 3600 + 1)) +
        0.5 * COALESCE(MAX(e.weight), 0)
            OVER (PARTITION BY n.session_id)
            AS relevance_score
    FROM graph_nodes n
    LEFT JOIN graph_edges e ON n.id = e.to_id AND e.edge_type = 'mentioned'
    WHERE n.session_id = :session_id
      AND n.focus_level <= 3          -- node_ceiling[1]
      AND n.timestamp > NOW() - INTERVAL '24 hours'  -- temporal_hours[1]
    GROUP BY n.id, n.type, n.content, n.timestamp, n.focus_level
)
SELECT * FROM lobby_nodes
ORDER BY relevance_score DESC
LIMIT 20;  -- max_results[1]
```

### Context Balloon Query (focus_level = 7)

Multi-dimensional subgraph extraction anchored at `current_seed`. BFS traversal with weighted scoring.

```sql
-- focus_level = 7: edge_threshold=0.5, node_ceiling=6, temporal_hours=168
WITH RECURSIVE balloon AS (
    -- Seed node (depth 0)
    SELECT
        n.id AS node_id,
        0 AS depth,
        1.0 AS edge_weight,
        1 - (:seed_embedding <=> n.embedding) AS semantic_score,
        1.0 / (EXTRACT(EPOCH FROM NOW() - n.timestamp) / 3600 + 1) AS temporal_score,
        ARRAY[n.id] AS path
    FROM graph_nodes n
    WHERE n.id = :current_seed
    
    UNION ALL
    
    -- Traverse edges (depth 1..3)
    SELECT
        target.id,
        balloon.depth + 1,
        edge.weight,
        1 - (target.embedding <=> :seed_embedding),
        1.0 / (EXTRACT(EPOCH FROM NOW() - target.timestamp) / 3600 + 1),
        balloon.path || target.id
    FROM balloon
    JOIN graph_edges edge ON balloon.node_id = edge.from_id
                           AND edge.weight >= 0.5           -- edge_threshold[7]
                           AND edge.edge_type IN (
                               'mentioned', 'similar_to',
                               'extracted_from', 'builds_on', 'parent'
                           )
    JOIN graph_nodes target ON edge.to_id = target.id
                           AND target.focus_level <= 6       -- node_ceiling[7]
                           AND target.timestamp > NOW() - INTERVAL '168 hours'  -- temporal_hours[7]
                           AND NOT target.id = ANY(balloon.path)  -- avoid cycles
    WHERE balloon.depth < 3
)
SELECT
    n.id,
    n.type,
    n.content,
    n.timestamp,
    b.depth,
    b.semantic_score,
    b.temporal_score,
    b.edge_weight,
    -- Combined relevance: weighted across all dimensions
    0.4 * b.semantic_score +        -- semantic dimension
    0.2 * b.temporal_score +        -- temporal dimension
    0.3 * b.edge_weight +           -- graph strength
    0.1 * (1.0 - b.depth / 3.0)     -- proximity bonus (closer = more relevant)
        AS combined_score
FROM balloon b
JOIN graph_nodes n ON b.node_id = n.id
ORDER BY combined_score DESC
LIMIT 80;  -- max_results[7]
```

### Semantic Similarity Query

Find nodes semantically similar to a seed, regardless of graph connectivity. Useful for "what else is like this?" exploration.

```sql
SELECT
    n.id,
    n.type,
    n.content,
    1 - (n.embedding <=> :query_embedding) AS similarity,
    n.timestamp
FROM graph_nodes n
WHERE n.embedding IS NOT NULL
  AND n.type IN ('fact', 'conversation', 'summary')
  -- Semantic distance threshold from focus config
  AND n.embedding <=> :query_embedding <= 0.3
ORDER BY n.embedding <=> :query_embedding
LIMIT 20;
```

### Temporal Window Query

All nodes within a time range, optionally filtered by type or session. Demonstrates PostGIS time-range capabilities.

```sql
-- Nodes from the last 7 days in a session
SELECT n.id, n.type, n.content, n.timestamp
FROM graph_nodes n
WHERE n.session_id = :session_id
  AND n.timestamp >= NOW() - INTERVAL '7 days'
  AND n.timestamp < NOW()
ORDER BY n.timestamp DESC;

-- Time-range overlap check (PostGIS tsrange)
-- Find nodes that overlap with a given time window
SELECT n.id, n.type, n.timestamp
FROM graph_nodes n
WHERE n.session_id = :session_id
  AND tsrange(n.timestamp, n.timestamp + INTERVAL '1 hour')
      OVERLAPS tsrange(:window_start, :window_end);
```

---

## Migration Notes

### From JSON to Graph: Mapping Current Structures

| Current JSON Structure | Graph Equivalent |
|-----------------------|-----------------|
| `working_memory.json` facts | `graph_nodes` with `type='fact'`, content `{key, value, source}` |
| Conversation turn (user+assistant) | `graph_nodes` with `type='conversation'`, content `{user_text, assistant_text, role, ordinal}` |
| Session metadata (id, name) | `sessions` table |
| Session history (linear array) | `graph_edges` with `type='follows'` linking consecutive conversation nodes |
| `tree` fact (disabled by default) | `graph_nodes` with `type='fact'`, `focus_level=2` (hidden from lobby) |
| `git_status` fact | `graph_nodes` with `type='fact'`, `focus_level=3` |

### Migration Script Outline

```sql
-- 1. Create sessions from existing session directories
INSERT INTO sessions (id, name, user_id, created_at, focus_level)
SELECT
    gen_random_uuid(),
    session_name,
    'default_user',
    created_timestamp,
    0  -- default focus level
FROM existing_session_metadata;

-- 2. Migrate working memory facts
INSERT INTO graph_nodes (session_id, type, content, focus_level, timestamp)
SELECT
    s.id,
    'fact',
    jsonb_build_object('key', f.key, 'value', f.value, 'source', 'user'),
    CASE
        WHEN f.key IN ('userid', 'home', 'os', 'arch') THEN 2  -- background, hidden from lobby
        WHEN f.key IN ('cwd', 'project', 'repo_root') THEN 4   -- project, visible at focus>=3
        WHEN f.key = 'tree' THEN 2                             -- disabled by default
        ELSE 5
    END,
    created_at
FROM sessions s
CROSS JOIN LATERAL jsonb_each_text(existing_wm->'facts') AS k(f);

-- 3. Migrate conversation history
INSERT INTO graph_nodes (session_id, type, content, focus_level, timestamp, ordinal)
SELECT
    s.id,
    'conversation',
    jsonb_build_object(
        'user_text', turn->>'user',
        'assistant_text', turn->>'assistant',
        'role', turn->>'role',
        'ordinal', turn->>'ordinal'
    ),
    5,
    turn->>'timestamp',
    (turn->>'ordinal')::int
FROM sessions s
CROSS JOIN LATERAL jsonb_array_elements(existing_convo) AS t(turn);

-- 4. Link consecutive conversations with 'follows' edges
INSERT INTO graph_edges (from_id, to_id, edge_type, weight)
SELECT
    prev.id, curr.id, 'follows', 1.0
FROM graph_nodes prev
JOIN graph_nodes curr ON curr.session_id = prev.session_id
    AND curr.content->>'ordinal' = ((prev.content->>'ordinal')::int + 1)::text
WHERE prev.type = 'conversation' AND curr.type = 'conversation';
```

---

## Next Steps (Detailed)

1. [ ] **Create the schema**: Run all CREATE TABLE/INDEX/TRIGGER statements above against the `agentx` database
2. [ ] **Seed focus_config**: Insert the default config row
3. [ ] **Prototype session load**: Replace one file-based `LoadWorkingMemory` call with a SQL query against `graph_nodes`
4. [ ] **Prototype graph traversal**: Implement the recursive CTE balloon query and verify it returns sensible results
5. [ ] **Prototype embedding generation**: Wire pgvector with an embedding API (Ollama local or OpenAI)
6. [ ] **Full session layer migration**: Replace all file I/O in the session package with database queries
7. [ ] **Implement lobby query**: Build the focus_level=0 query path
8. [ ] **Implement focus transitions**: Add logic to adjust focus_level on the session based on user input
9. [ ] **Testing**: Unit tests, integration tests, performance benchmarks
10. [ ] **Cutover**: Deploy with dual-write during transition, then switch to database-only

## Open Questions

- [ ] Should `graph_edges` support metadata (extra JSONB fields beyond weight)?
- [ ] How to handle concurrent session focus changes (lock strategy)?
- [ ] Should semantic edges be pre-computed (on node insert) or computed on query (lazy)?
- [ ] What embedding model to use? (OpenAI text-embedding-3-small vs. local Ollama nomic-embed)
- [ ] How to handle node deletion (hard delete vs. soft delete with `deleted_at`)?
- [ ] Should the lobby be computed server-side or client-side from raw node data?
- [ ] How to version the schema for future migrations?

---

*Document created: 2025-01-15. Last updated: 2025-01-15.*
*Status: Schema design complete. Ready for implementation.*
