const overviewURL = "./api/overview";
const refreshMs = 2000;

const summaryGrid = document.querySelector("#summary-grid");
const topology = document.querySelector("#topology");
const shardMap = document.querySelector("#shard-map");
const groupGrid = document.querySelector("#group-grid");
const migrationPanel = document.querySelector("#migration-panel");
const errorPanel = document.querySelector("#error-panel");
const generatedAt = document.querySelector("#generated-at");
const emptyTemplate = document.querySelector("#empty-state-template");

let pollHandle = null;

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function cloneEmptyState(message) {
  const fragment = emptyTemplate.content.cloneNode(true);
  fragment.querySelector("p").textContent = message;
  return fragment;
}

function statusClass(status) {
  switch (status) {
    case "healthy":
    case "online":
      return "status-healthy";
    case "migrating":
    case "degraded":
      return "status-migrating";
    case "offline":
      return "status-offline";
    default:
      return "status-unknown";
  }
}

function renderSummary(summary) {
  const cards = [
    ["Config Version", summary.config_version, `Shards ${summary.total_shards}`],
    ["Nodes Online", `${summary.reachable_nodes}/${summary.total_nodes}`, "Reachable / Total"],
    ["Shard Nodes", summary.shard_nodes, "Data plane replicas"],
    ["Control Plane", `${summary.controller_nodes + summary.apiserver_nodes}`, `Controller ${summary.controller_nodes} / API ${summary.apiserver_nodes}`],
    ["Groups", summary.group_count, `Healthy ${summary.healthy_groups}`],
    ["Migrating", summary.migrating_groups, "Groups with locked shards"],
  ];

  summaryGrid.innerHTML = cards.map(([label, value, hint]) => `
    <article class="card summary-card">
      <span class="label">${label}</span>
      <div class="value">${value}</div>
      <div class="hint">${hint}</div>
    </article>
  `).join("");
}

function renderTopology(nodes) {
  const roleOrder = ["controller", "apiserver", "shard"];
  const roleTitles = {
    controller: "Controller",
    apiserver: "API Server",
    shard: "Shard Nodes",
  };

  topology.innerHTML = "";
  roleOrder.forEach((role) => {
    const roleNodes = nodes.filter((node) => node.role === role);
    if (!roleNodes.length) {
      return;
    }

    const section = document.createElement("section");
    section.className = "role-section";
    section.innerHTML = `<h3>${roleTitles[role]}</h3>`;

    const grid = document.createElement("div");
    grid.className = "node-grid";
    roleNodes.forEach((node) => {
      const card = document.createElement("article");
      card.className = "node-card";
      card.innerHTML = `
        <div class="node-head">
          <div>
            <div class="node-id">${node.id}</div>
            <div class="node-role">${node.group_id || "control"} ${node.is_leader ? "· leader" : ""}</div>
          </div>
          <span class="status-badge ${statusClass(node.status)}">${node.status}</span>
        </div>
        <div class="node-meta">
          <div><strong>Role:</strong> ${node.role}</div>
          <div><strong>HTTP:</strong> <span class="mono">${node.http_addr || "-"}</span></div>
          <div><strong>Raft:</strong> <span class="mono">${node.raft_addr || "-"}</span></div>
          <div><strong>Cluster Leader:</strong> <span class="mono">${node.cluster_leader || "-"}</span></div>
          <div><strong>Tables:</strong> ${node.table_count || 0}</div>
          ${node.last_error ? `<div><strong>Error:</strong> ${node.last_error}</div>` : ""}
        </div>
      `;
      grid.appendChild(card);
    });

    section.appendChild(grid);
    topology.appendChild(section);
  });

  if (!topology.children.length) {
    topology.appendChild(cloneEmptyState("未发现任何节点，请确认 apiserver 已连接 discovery。"));
  }
}

function renderShards(shards, lockedShards) {
  shardMap.innerHTML = "";
  const lockedSet = new Set(asArray(lockedShards));
  const assignments = asArray(shards?.assignments);
  if (!assignments.length) {
    shardMap.appendChild(cloneEmptyState("暂无 shard 分配信息。"));
    return;
  }

  assignments.forEach((item) => {
    const card = document.createElement("article");
    card.className = `shard-card ${lockedSet.has(item.shard_id) ? "migrating" : ""}`;
    card.innerHTML = `
      <div class="title">Shard ${item.shard_id}</div>
      <div class="group">${item.group_id}</div>
      <div class="hint">${lockedSet.has(item.shard_id) ? "Migrating" : "Stable"}</div>
    `;
    shardMap.appendChild(card);
  });
}

function renderGroups(groups) {
  groupGrid.innerHTML = "";
  const safeGroups = asArray(groups);
  if (!safeGroups.length) {
    groupGrid.appendChild(cloneEmptyState("暂无 Group 状态。"));
    return;
  }

  safeGroups.forEach((group) => {
    const shards = asArray(group.shards);
    const nodes = asArray(group.nodes);
    const card = document.createElement("article");
    card.className = "group-card";
    card.innerHTML = `
      <div class="group-head">
        <div class="group-name">${group.group_id}</div>
        <span class="status-badge ${statusClass(group.status)}">${group.status}</span>
      </div>
      <div class="group-meta">
        <span class="pill">Shards ${group.shard_count}</span>
        <span class="pill">Nodes ${group.reachable_nodes}/${group.node_count}</span>
        <span class="pill">Leader ${group.leader_node_id || "-"}</span>
      </div>
      <div class="chip-row">
        ${shards.map((shard) => `<span class="chip">shard-${shard}</span>`).join("") || `<span class="chip">no shards</span>`}
      </div>
      <div class="chip-row" style="margin-top:12px;">
        ${nodes.map((node) => `<span class="tag ${statusClass(node.status)}">${node.id}${node.is_leader ? " · leader" : ""}</span>`).join("") || `<span class="chip">no nodes</span>`}
      </div>
    `;
    groupGrid.appendChild(card);
  });
}

function renderMigration(lockedShards) {
  migrationPanel.innerHTML = "";
  const safeLockedShards = asArray(lockedShards);
  const card = document.createElement("div");
  card.className = "alert-card";
  card.innerHTML = `<h3>Locked Shards</h3>`;

  const row = document.createElement("div");
  row.className = "chip-row";
  if (!safeLockedShards.length) {
    row.appendChild(cloneEmptyState("当前没有正在迁移的 shard。"));
  } else {
    safeLockedShards.forEach((shardID) => {
      const chip = document.createElement("span");
      chip.className = "tag status-migrating";
      chip.textContent = `shard-${shardID}`;
      row.appendChild(chip);
    });
  }
  card.appendChild(row);
  migrationPanel.appendChild(card);
}

function renderErrors(errors) {
  errorPanel.innerHTML = "";
  const card = document.createElement("div");
  card.className = "alert-card";
  card.innerHTML = `<h3>Aggregator Alerts</h3>`;
  const list = document.createElement("div");
  list.className = "alert-list";

  if (!errors || !errors.length) {
    list.appendChild(cloneEmptyState("当前没有聚合错误。"));
  } else {
    errors.forEach((message) => {
      const item = document.createElement("div");
      item.className = "error-item";
      item.textContent = message;
      list.appendChild(item);
    });
  }

  card.appendChild(list);
  errorPanel.appendChild(card);
}

function renderOverview(overview) {
  generatedAt.textContent = new Date(overview.generated_at).toLocaleString();
  renderSummary(overview.summary);
  renderTopology(overview.nodes || []);
  renderShards(overview.shards || { assignments: [] }, overview.locked_shards || []);
  renderGroups(overview.groups || []);
  renderMigration(overview.locked_shards || []);
  renderErrors(overview.errors || []);
}

async function loadOverview() {
  const response = await fetch(overviewURL, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`dashboard overview request failed: ${response.status}`);
  }
  return response.json();
}

async function tick() {
  try {
    const overview = await loadOverview();
    renderOverview(overview);
  } catch (error) {
    renderErrors([error.message || String(error)]);
    generatedAt.textContent = `Refresh failed: ${new Date().toLocaleTimeString()}`;
  }
}

function startPolling() {
  tick();
  pollHandle = window.setInterval(tick, refreshMs);
}

window.addEventListener("beforeunload", () => {
  if (pollHandle) {
    window.clearInterval(pollHandle);
  }
});

startPolling();
