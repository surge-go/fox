(function () {
  "use strict";

  var UI_BUILD = "20260525-file-tabs";
  var MAX_UPLOAD_FILE_BYTES = 32 * 1024 * 1024;

  var state = {
    config: readConfig(),
    spec: null,
    operations: [],
    filtered: [],
    selectedKey: "",
    serverURL: "",
    environmentModalOpen: false,
    environmentDraftURL: "",
    globalHeaderSuggestionsOpen: false,
    globalHeaderBatchOpen: false,
    globalHeaderBatchMode: "colon",
    globalHeaderBatchDraft: "",
    globalHeaders: [],
    collapsedGroups: {},
    methodFilter: "",
    requestTabByOperation: {},
    responseTabByOperation: {},
    responseResultByOperation: {},
    responseViewByOperation: {},
    validateResponseByOperation: {},
    parameterValuesByOperation: {},
    headersByOperation: {},
    headerSuggestionsOpenByOperation: {},
    bodyExamplesByOperation: {},
    bodyFilesByOperation: {}
  };

  var els = {};
  var COMMON_HEADERS = [
    "Accept",
    "Accept-Charset",
    "Accept-Encoding",
    "Accept-Language",
    "Cache-Control",
    "Content-MD5",
    "If-Match",
    "If-Modified-Since",
    "If-None-Match",
    "Origin",
    "Referer",
    "User-Agent",
    "X-API-Key",
    "X-Request-ID"
  ];
  var HEADER_TYPES = ["string", "integer", "number", "boolean", "array", "object"];

  document.addEventListener("DOMContentLoaded", function () {
    console.info("OpenAPI UI build", UI_BUILD);
    els.title = document.getElementById("doc-title");
    els.meta = document.getElementById("doc-meta");
    els.server = document.getElementById("server-select");
    els.envBadge = document.getElementById("env-current-badge");
    els.envLabel = document.getElementById("env-current-label");
    els.envManage = document.getElementById("env-manage");
    els.theme = document.getElementById("theme-toggle");
    els.summary = document.getElementById("summary-strip");
    els.list = document.getElementById("operation-list");
    els.tabs = document.getElementById("workspace-tabs");
    els.content = document.getElementById("content");

    initTheme();
    bindEvents();
    loadSpec();
  });

  function readConfig() {
    var node = document.getElementById("openapi-ui-config");
    if (!node) {
      return { title: "OpenAPI 工作台", specUrl: "openapi.json" };
    }
    try {
      var config = JSON.parse(node.textContent || "{}");
      if (!config.title || config.title.indexOf("__OPENAPI") >= 0) {
        config.title = "OpenAPI 工作台";
      }
      if (!config.specUrl || config.specUrl.indexOf("__OPENAPI") >= 0) {
        config.specUrl = "openapi.json";
      }
      return config;
    } catch (err) {
      return { title: "OpenAPI 工作台", specUrl: "openapi.json" };
    }
  }

  function initTheme() {
    var key = storageKey();
    var saved = localStorage.getItem(key);
    if (saved === "light" || saved === "dark") {
      document.documentElement.dataset.theme = saved;
      updateThemeButton(saved);
      return;
    }
    document.documentElement.dataset.theme = "dark";
    updateThemeButton("dark");
  }

  function storageKey() {
    return state.config.storagePrefix ? state.config.storagePrefix + "-theme" : "openapi-ui-theme";
  }

  function bindEvents() {
    els.theme.addEventListener("click", function () {
      var current = document.documentElement.dataset.theme || "dark";
      var next = current === "dark" ? "light" : "dark";
      document.documentElement.dataset.theme = next;
      localStorage.setItem(storageKey(), next);
      updateThemeButton(next);
    });

    els.server.addEventListener("change", function () {
      state.serverURL = els.server.value;
      updateServerDisplay();
      renderSelectedOperation();
    });

    if (els.envManage) {
      els.envManage.addEventListener("click", function () {
        openEnvironmentModal();
      });
    }

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && state.globalHeaderBatchOpen) {
        closeGlobalHeaderBatchModal();
        return;
      }
      if (event.key === "Escape" && state.environmentModalOpen) {
        closeEnvironmentModal();
      }
    });
  }

  function applySearchFilter() {
    var query = "";
    if (els.search) {
      query = els.search.value.trim().toLowerCase();
    }
    state.filtered = state.operations.filter(function (op) {
      var matchesQuery = !query || [op.path, op.method, op.summary, op.operationId, op.tag].join(" ").toLowerCase().indexOf(query) >= 0;
      var matchesMethod = !state.methodFilter || op.method === state.methodFilter;
      return matchesQuery && matchesMethod;
    });
  }

  function nextMethodFilter(current) {
    var sequence = ["", "get", "post", "put", "patch", "delete"];
    var index = sequence.indexOf(current);
    return sequence[(index + 1) % sequence.length];
  }

  function updateFilterButton() {
    var button = document.querySelector('[aria-label="筛选"]');
    if (!button) {
      return;
    }
    button.classList.toggle("active", Boolean(state.methodFilter));
    button.title = state.methodFilter ? "筛选：" + state.methodFilter.toUpperCase() : "筛选";
  }

  async function loadSpec() {
    try {
      var response = await fetch(state.config.specUrl, { headers: { "Accept": "application/json" } });
      if (!response.ok) {
        throw new Error("加载 " + state.config.specUrl + " 时返回 HTTP " + response.status);
      }
      applySpec(await response.json());
    } catch (err) {
      if (isLocalFilePage()) {
        applySpec(demoSpec());
        return;
      }
      renderError(err);
    }
  }

  function applySpec(spec) {
    state.spec = spec;
    state.operations = collectOperations(state.spec);
    state.filtered = state.operations.slice();
    state.serverURL = firstServerURL(state.spec);
    state.selectedKey = "";
    state.methodFilter = "";
    state.requestTabByOperation = {};
    renderShell();
  }

  function isLocalFilePage() {
    return window.location && window.location.protocol === "file:";
  }

  function renderShell() {
    var spec = state.spec;
    document.title = spec.info && spec.info.title ? spec.info.title : state.config.title;
    els.title.textContent = spec.info && spec.info.title ? spec.info.title : "OpenAPI 工作台";
    els.meta.innerHTML = specSummaryMarkup(spec);

    renderServers();
    renderSummary();
    renderOperationList();
    renderSelectedOperation();
  }

  function renderServers() {
    var servers = state.spec.servers || [];
    els.server.innerHTML = "";
    if (!servers.length) {
      var option = document.createElement("option");
      option.value = "";
      option.textContent = "暂无环境";
      els.server.appendChild(option);
      els.server.disabled = true;
      updateServerDisplay();
      return;
    }
    els.server.disabled = false;
    servers.forEach(function (server) {
      var option = document.createElement("option");
      option.value = server.url || "";
      option.textContent = environmentName(server);
      els.server.appendChild(option);
    });
    if (state.serverURL && !servers.some(function (server) { return (server.url || "") === state.serverURL; })) {
      var custom = document.createElement("option");
      custom.value = state.serverURL;
      custom.textContent = "自定义环境";
      els.server.appendChild(custom);
    }
    els.server.value = state.serverURL;
    updateServerDisplay();
  }

  function updateServerDisplay() {
    if (!els.envBadge || !els.envLabel || !els.server) {
      return;
    }
    var option = els.server.options[els.server.selectedIndex];
    var label = option && option.textContent ? option.textContent : "暂无环境";
    els.envLabel.textContent = label;
    els.envBadge.textContent = label.slice(0, 1) || "环";
  }

  function openEnvironmentModal() {
    state.environmentDraftURL = state.serverURL || firstServerURL(state.spec || {});
    state.environmentModalOpen = true;
    renderEnvironmentModal(true);
  }

  function closeEnvironmentModal() {
    state.globalHeaderBatchOpen = false;
    state.environmentModalOpen = false;
    renderEnvironmentModal();
  }

  function applyEnvironmentModal() {
    state.serverURL = (state.environmentDraftURL || "").trim();
    renderServers();
    renderSelectedOperation();
    closeEnvironmentModal();
  }

  function renderEnvironmentModal(focusURL) {
    var existing = document.getElementById("environment-modal");
    if (existing) {
      existing.remove();
    }
    if (!state.environmentModalOpen) {
      return;
    }

    var servers = state.spec && state.spec.servers ? state.spec.servers : [];
    var selectedURL = state.environmentDraftURL || firstServerURL(state.spec || {});
    var selectedServer = servers.find(function (server) {
      return (server.url || "") === selectedURL;
    }) || { url: selectedURL, description: selectedURL ? "自定义环境" : "暂无环境" };

    var modal = document.createElement("div");
    modal.id = "environment-modal";
    modal.className = "env-modal";
    modal.innerHTML =
      '<div class="env-modal-backdrop" data-env-close></div>' +
      '<section class="env-dialog" role="dialog" aria-modal="true" aria-labelledby="env-dialog-title">' +
      '<header class="env-dialog-head">' +
      '<h2 id="env-dialog-title">环境管理</h2>' +
      '<button class="env-close" type="button" aria-label="关闭环境管理" data-env-close>' + icon("x") + '</button>' +
      '</header>' +
      '<div class="env-dialog-body">' +
      '<aside class="env-nav" aria-label="环境导航">' +
      '<div class="env-nav-section"><span>环境</span>' +
      renderEnvironmentNav(servers, selectedURL) +
      '</div>' +
      '</aside>' +
      '<main class="env-detail">' +
      '<div class="env-detail-toolbar">' +
      '<div class="env-title-group"><span class="env-badge">' + escapeHTML(environmentBadge(selectedServer)) + '</span><strong>' + escapeHTML(environmentName(selectedServer)) + '</strong></div>' +
      '</div>' +
      '<section class="env-section">' +
      '<h3>前置 URL</h3>' +
      '<div class="env-url-grid" role="table" aria-label="前置 URL">' +
      '<div class="env-grid-head" role="row"><span role="columnheader">模块</span><span role="columnheader">前置 URL</span></div>' +
      '<div class="env-grid-row" role="row"><span role="cell">默认模块</span><label class="sr-only" for="env-url-input">前置 URL</label><input id="env-url-input" class="env-url-input" value="' + escapeAttribute(selectedURL) + '" placeholder="http://localhost:8080" data-env-url /></div>' +
      '</div>' +
      '</section>' +
      '<section class="env-section">' +
      '<div class="env-param-tabs" role="tablist" aria-label="参数类型">' +
      '<div class="env-param-tab active" role="tab" aria-selected="true">Header' + (activeGlobalHeaderCount() ? '<span class="count-dot">' + activeGlobalHeaderCount() + '</span>' : "") + '</div>' +
      '</div>' +
      renderGlobalParamPanel() +
      '</section>' +
      '<footer class="env-dialog-foot">' +
      '<button class="button secondary compact" type="button" data-env-close>取消</button>' +
      '<button class="button primary compact" type="button" data-env-apply>应用当前环境</button>' +
      '</footer>' +
      '</main>' +
      '</div>' +
      '</section>' +
      renderGlobalHeaderBatchModal();

    document.body.appendChild(modal);
    bindEnvironmentModalActions(modal);
    var input = modal.querySelector("[data-env-url]");
    if (focusURL && input) {
      input.focus({ preventScroll: true });
      input.select();
    }
  }

  function bindEnvironmentModalActions(modal) {
    Array.prototype.forEach.call(modal.querySelectorAll("[data-env-close]"), function (button) {
      button.addEventListener("click", closeEnvironmentModal);
    });
    Array.prototype.forEach.call(modal.querySelectorAll("[data-env-index]"), function (button) {
      button.addEventListener("click", function () {
        var servers = state.spec && state.spec.servers ? state.spec.servers : [];
        var server = servers[Number(button.dataset.envIndex)];
        state.environmentDraftURL = server && server.url ? server.url : "";
        renderEnvironmentModal();
      });
    });
    var input = modal.querySelector("[data-env-url]");
    if (input) {
      input.addEventListener("input", function () {
        state.environmentDraftURL = input.value;
      });
    }
    var apply = modal.querySelector("[data-env-apply]");
    if (apply) {
      apply.addEventListener("click", applyEnvironmentModal);
    }
    Array.prototype.forEach.call(modal.querySelectorAll("[data-env-add-variable]"), function (button) {
      button.addEventListener("click", function () {
        showToast("环境配置已在本地弹窗中管理");
      });
    });
    Array.prototype.forEach.call(modal.querySelectorAll("[data-global-header-field]"), function (input) {
      input.addEventListener("input", function () {
        var index = Number(input.dataset.globalHeaderIndex);
        var field = input.dataset.globalHeaderField;
        var header = state.globalHeaders[index];
        if (!header || !field) {
          return;
        }
        header[field] = input.value;
        if (field === "key" && !input.value.trim()) {
          state.globalHeaderSuggestionsOpen = true;
          renderEnvironmentModal();
        }
      });
      input.addEventListener("change", function () {
        var index = Number(input.dataset.globalHeaderIndex);
        var field = input.dataset.globalHeaderField;
        var header = state.globalHeaders[index];
        if (!header || !field) {
          return;
        }
        header[field] = input.value;
      });
    });
    Array.prototype.forEach.call(modal.querySelectorAll("[data-global-header-enabled]"), function (button) {
      button.addEventListener("click", function () {
        var header = state.globalHeaders[Number(button.dataset.globalHeaderEnabled)];
        if (!header) {
          return;
        }
        header.enabled = header.enabled === false;
        renderEnvironmentModal();
      });
    });
    Array.prototype.forEach.call(modal.querySelectorAll("[data-global-header-remove]"), function (button) {
      button.addEventListener("click", function () {
        var index = Number(button.dataset.globalHeaderRemove);
        state.globalHeaders.splice(index, 1);
        renderEnvironmentModal();
      });
    });
    var addHeader = modal.querySelector("[data-global-header-add]");
    if (addHeader) {
      addHeader.addEventListener("click", function () {
        addBlankGlobalHeader();
        renderEnvironmentModal();
      });
    }
    Array.prototype.forEach.call(modal.querySelectorAll("[data-global-header-suggestion]"), function (button) {
      button.addEventListener("click", function () {
        applyGlobalHeaderSuggestion(button.dataset.globalHeaderSuggestion || "");
        renderEnvironmentModal();
      });
    });
    var batchOpen = modal.querySelector("[data-global-header-batch-open]");
    if (batchOpen) {
      batchOpen.addEventListener("click", openGlobalHeaderBatchModal);
    }
    Array.prototype.forEach.call(modal.querySelectorAll("[data-global-header-batch-close]"), function (button) {
      button.addEventListener("click", closeGlobalHeaderBatchModal);
    });
    Array.prototype.forEach.call(modal.querySelectorAll("[data-global-header-batch-mode]"), function (button) {
      button.addEventListener("click", function () {
        state.globalHeaderBatchMode = button.dataset.globalHeaderBatchMode || "colon";
        state.globalHeaderBatchDraft = serializeGlobalHeadersForBatch(state.globalHeaderBatchMode);
        renderEnvironmentModal();
      });
    });
    var batchText = modal.querySelector("[data-global-header-batch-text]");
    if (batchText) {
      batchText.addEventListener("input", function () {
        state.globalHeaderBatchDraft = batchText.value;
      });
      batchText.focus({ preventScroll: true });
    }
    var batchApply = modal.querySelector("[data-global-header-batch-apply]");
    if (batchApply) {
      batchApply.addEventListener("click", applyGlobalHeaderBatchModal);
    }
  }

  function activeGlobalHeaderCount() {
    return state.globalHeaders.filter(function (header) {
      return (header.key || "").trim();
    }).length;
  }

  function renderGlobalParamPanel() {
    return '<div class="env-global-header-grid" role="table" aria-label="全局 Header">' +
      '<div class="env-global-header-head" role="row"><span></span><span role="columnheader">参数名</span><span role="columnheader">类型</span><span role="columnheader">默认值</span><span role="columnheader">默认启用</span><span role="columnheader">说明</span><button class="env-batch-open" type="button" data-global-header-batch-open>批量编辑</button></div>' +
      state.globalHeaders.map(renderGlobalHeaderRow).join("") +
      renderGlobalHeaderSuggestions() +
      '<button class="env-empty-row" type="button" data-global-header-add>添加参数</button>' +
      '</div>';
  }

  function renderGlobalHeaderSuggestions() {
    var hasBlank = state.globalHeaders.some(function (header) {
      return !(header.key || "").trim();
    });
    if (!state.globalHeaderSuggestionsOpen && !hasBlank) {
      return "";
    }
    var used = {};
    state.globalHeaders.forEach(function (header) {
      if ((header.key || "").trim()) {
        used[header.key.trim().toLowerCase()] = true;
      }
    });
    var suggestions = COMMON_HEADERS.filter(function (name) {
      return !used[name.toLowerCase()];
    });
    if (!suggestions.length) {
      return '<div class="env-global-header-suggestions"><div class="header-suggestion-menu"><span class="header-suggestion-empty">暂无更多常用 Header</span></div></div>';
    }
    return '<div class="env-global-header-suggestions"><div class="header-suggestion-menu">' + suggestions.map(function (name) {
      return '<button class="header-suggestion" type="button" data-global-header-suggestion="' + escapeAttribute(name) + '">' + escapeHTML(name) + '</button>';
    }).join("") + '</div></div>';
  }

  function addBlankGlobalHeader() {
    for (var i = 0; i < state.globalHeaders.length; i++) {
      if (!(state.globalHeaders[i].key || "").trim()) {
        state.globalHeaderSuggestionsOpen = true;
        return;
      }
    }
    state.globalHeaders.push({ key: "", value: "", type: "string", description: "", enabled: true });
    state.globalHeaderSuggestionsOpen = true;
  }

  function applyGlobalHeaderSuggestion(key) {
    if (!key) {
      return;
    }
    var target = null;
    for (var i = 0; i < state.globalHeaders.length; i++) {
      if (!(state.globalHeaders[i].key || "").trim()) {
        target = state.globalHeaders[i];
        break;
      }
    }
    if (!target) {
      target = { key: "", value: "", type: "string", description: "", enabled: true };
      state.globalHeaders.push(target);
    }
    target.key = key;
    target.type = "string";
    target.description = headerDescription(key);
    target.enabled = true;
    state.globalHeaderSuggestionsOpen = false;
  }

  function openGlobalHeaderBatchModal() {
    state.globalHeaderBatchMode = state.globalHeaderBatchMode || "colon";
    state.globalHeaderBatchDraft = serializeGlobalHeadersForBatch(state.globalHeaderBatchMode);
    state.globalHeaderBatchOpen = true;
    renderEnvironmentModal();
  }

  function closeGlobalHeaderBatchModal() {
    state.globalHeaderBatchOpen = false;
    renderEnvironmentModal();
  }

  function applyGlobalHeaderBatchModal() {
    var parsed = parseGlobalHeaderBatch(state.globalHeaderBatchDraft, state.globalHeaderBatchMode);
    if (!parsed.length) {
      showToast("没有可应用的 Header");
      return;
    }
    var existingByKey = {};
    state.globalHeaders.forEach(function (header) {
      if ((header.key || "").trim()) {
        existingByKey[header.key.trim().toLowerCase()] = header;
      }
    });
    state.globalHeaders = parsed.map(function (item) {
      var existing = existingByKey[item.key.toLowerCase()] || {};
      return {
        key: item.key,
        value: item.value,
        type: existing.type || "string",
        description: existing.description || headerDescription(item.key),
        enabled: existing.enabled !== false
      };
    });
    state.globalHeaderBatchOpen = false;
    renderEnvironmentModal();
  }

  function renderGlobalHeaderBatchModal() {
    if (!state.globalHeaderBatchOpen) {
      return "";
    }
    var mode = state.globalHeaderBatchMode || "colon";
    return '<div class="env-batch-modal" role="dialog" aria-modal="true" aria-labelledby="env-batch-title">' +
      '<div class="env-batch-backdrop" data-global-header-batch-close></div>' +
      '<section class="env-batch-dialog">' +
      '<header class="env-batch-head"><h3 id="env-batch-title">批量编辑</h3><button class="env-close" type="button" aria-label="关闭批量编辑" data-global-header-batch-close>' + icon("x") + '</button></header>' +
      '<div class="env-batch-tools">' +
      '<button class="env-mode-button' + (mode === "comma" ? " active" : "") + '" type="button" data-global-header-batch-mode="comma">逗号模式</button>' +
      '<button class="env-mode-button' + (mode === "colon" ? " active" : "") + '" type="button" data-global-header-batch-mode="colon">冒号模式</button>' +
      '<span>格式：参数名' + (mode === "comma" ? "," : ":") + '默认值</span>' +
      '</div>' +
      '<label class="sr-only" for="env-batch-text">批量 Header</label>' +
      '<textarea id="env-batch-text" class="env-batch-textarea" data-global-header-batch-text spellcheck="false">' + escapeHTML(state.globalHeaderBatchDraft) + '</textarea>' +
      '<p class="env-batch-hint">字段之间以英文' + (mode === "comma" ? "逗号（,）" : "冒号（:）") + '分隔，多条记录以换行分隔</p>' +
      '<footer class="env-batch-foot"><button class="button secondary compact" type="button" data-global-header-batch-close>取消</button><button class="button primary compact" type="button" data-global-header-batch-apply>确定</button></footer>' +
      '</section>' +
      '</div>';
  }

  function serializeGlobalHeadersForBatch(mode) {
    var separator = mode === "comma" ? "," : ":";
    return state.globalHeaders.filter(function (header) {
      return (header.key || "").trim();
    }).map(function (header) {
      return header.key.trim() + separator + (header.value || "");
    }).join("\n");
  }

  function parseGlobalHeaderBatch(text, mode) {
    var separator = mode === "comma" ? "," : ":";
    return String(text || "").split(/\r?\n/).map(function (line) {
      var trimmed = line.trim();
      if (!trimmed) {
        return null;
      }
      var index = trimmed.indexOf(separator);
      var key = index >= 0 ? trimmed.slice(0, index).trim() : trimmed;
      var value = index >= 0 ? trimmed.slice(index + 1).trim() : "";
      if (!key) {
        return null;
      }
      return { key: key, value: value };
    }).filter(Boolean);
  }

  function renderGlobalHeaderRow(header, index) {
    var disabled = header.enabled === false ? " unchecked" : "";
    return '<div class="env-global-header-row" role="row" data-global-header-index="' + index + '">' +
      '<span class="env-drag-handle" aria-hidden="true">' + icon("grip") + '</span>' +
      '<label class="sr-only" for="global-header-key-' + index + '">参数名</label>' +
      '<input id="global-header-key-' + index + '" class="env-header-input" data-global-header-index="' + index + '" data-global-header-field="key" value="' + escapeAttribute(header.key || "") + '" placeholder="Header 名称" />' +
      '<select class="env-header-select" data-global-header-index="' + index + '" data-global-header-field="type">' + headerTypeOptions(header.type || "string") + '</select>' +
      '<label class="sr-only" for="global-header-value-' + index + '">默认值</label>' +
      '<input id="global-header-value-' + index + '" class="env-header-input" data-global-header-index="' + index + '" data-global-header-field="value" value="' + escapeAttribute(header.value || "") + '" placeholder="Header 默认值" />' +
      '<button class="env-header-switch' + disabled + '" type="button" data-global-header-enabled="' + index + '" aria-pressed="' + (header.enabled === false ? "false" : "true") + '"><span></span></button>' +
      '<label class="sr-only" for="global-header-description-' + index + '">说明</label>' +
      '<input id="global-header-description-' + index + '" class="env-header-input" data-global-header-index="' + index + '" data-global-header-field="description" value="' + escapeAttribute(header.description || "") + '" placeholder="说明" />' +
      '<button class="env-row-remove" type="button" data-global-header-remove="' + index + '" aria-label="移除 Header">' + icon("minus") + '</button>' +
      '</div>';
  }

  function renderEnvironmentNav(servers, selectedURL) {
    if (!servers.length) {
      return '<div class="env-nav-empty">暂无环境</div>';
    }
    return servers.map(function (server, index) {
      var active = (server.url || "") === selectedURL ? " active" : "";
      return '<button class="env-nav-item' + active + '" type="button" data-env-index="' + index + '">' +
        '<span class="env-mini-badge">' + escapeHTML(environmentBadge(server)) + '</span>' +
        '<span>' + escapeHTML(environmentName(server)) + '</span>' +
        '</button>';
    }).join("");
  }

  function environmentName(server) {
    return server && server.description ? server.description : (server && server.url ? server.url : "未命名环境");
  }

  function environmentBadge(server) {
    var name = environmentName(server);
    return name.slice(0, 1) || "环";
  }

  function renderSummary() {
    els.summary.innerHTML = "";
  }

  function specSummaryMarkup(spec) {
    var info = spec && spec.info ? spec.info : {};
    var parts = [];
    if (info.description) {
      parts.push('<span class="doc-summary">' + escapeHTML(info.description) + '</span>');
    }
    if (info.version) {
      parts.push('<span class="doc-badge">v' + escapeHTML(info.version) + '</span>');
    }
    if (spec && spec.openapi) {
      parts.push('<span class="doc-badge">OpenAPI ' + escapeHTML(spec.openapi) + '</span>');
    }
    return parts.join("");
  }

  function renderOperationList() {
    els.list.innerHTML = "";
    if (!state.filtered.length) {
      var empty = document.createElement("div");
      empty.className = "empty-state";
      empty.innerHTML = "<h2>没有匹配的接口</h2><p>调整搜索内容后查看更多路径。</p>";
      els.list.appendChild(empty);
      return;
    }

    buildOperationTree(state.filtered).children.forEach(function (node) {
      els.list.appendChild(renderOperationGroup(node, 0));
    });

    bindOperationListActions();
  }

  function bindOperationListActions() {
    Array.prototype.forEach.call(els.list.querySelectorAll("[data-group-key]"), function (heading) {
      heading.addEventListener("click", function () {
        var key = heading.dataset.groupKey || "";
        state.collapsedGroups[key] = !state.collapsedGroups[key];
        renderOperationList();
      });
    });
  }

  function renderOperationGroup(node, level) {
    var section = document.createElement("section");
    section.className = "tag-group" + (state.collapsedGroups[node.key] ? " collapsed" : "");
    section.style.setProperty("--level", String(level));

    var heading = document.createElement("button");
    heading.type = "button";
    heading.className = "tag-heading";
    heading.dataset.groupKey = node.key;
    heading.innerHTML = '<span class="chevron"></span>' + icon("folder") + '<span>' + escapeHTML(node.name) + '</span><small>(' + node.count + ')</small>';
    section.appendChild(heading);

    var children = document.createElement("div");
    children.className = "tag-children";
    node.children.forEach(function (child) {
      children.appendChild(renderOperationGroup(child, level + 1));
    });
    node.operations.forEach(function (op) {
      children.appendChild(renderOperationItem(op, level + 1));
    });
    section.appendChild(children);
    return section;
  }

  function renderOperationItem(op, level) {
    var button = document.createElement("button");
    button.type = "button";
    button.className = "op-item" + (op.key === state.selectedKey ? " active" : "");
    button.dataset.key = op.key;
    button.title = (op.summary || op.operationId || op.path) + "  " + op.path;
    button.style.setProperty("--level", String(level));
    button.innerHTML =
      '<span class="method"></span>' +
      '<span class="op-copy"><span class="op-summary"></span><span class="op-path"></span></span>' +
      '<span class="op-more" aria-hidden="true">...</span>';
    button.querySelector(".method").className = "method " + methodClass(op.method);
    button.querySelector(".method").textContent = op.method.toUpperCase();
    button.querySelector(".op-path").textContent = op.path;
    button.querySelector(".op-summary").textContent = op.summary || op.operationId || "暂无摘要";
    button.addEventListener("click", function () {
      selectOperation(op.key);
      els.content.focus({ preventScroll: true });
    });
    return button;
  }

  function selectOperation(key) {
    if (!key || key === state.selectedKey) {
      renderSelectedOperation();
      return;
    }
    state.selectedKey = key;
    renderSelectedOperation();
  }

  function renderWorkspaceTabs() {
    renderWorkspaceTabFor(selectedOperation());
  }

  function renderWorkspaceTabFor(op) {
    if (!op) {
      els.tabs.innerHTML = "";
      return;
    }
    els.tabs.innerHTML =
      '<div class="workspace-tab active" data-workspace-key="' + escapeAttribute(op.key) + '"><span class="method ' + methodClass(op.method) + '">' + op.method.toUpperCase() + '</span><span class="workspace-tab-title">' +
      escapeHTML(op.summary || op.path) + '</span><button class="tab-close" type="button" aria-label="关闭标签">x</button></div>';
    bindWorkspaceTabActions();
  }

  function bindWorkspaceTabActions() {
    var close = els.tabs.querySelector(".tab-close");
    if (close) {
      close.addEventListener("click", function () {
        showToast("至少保留一个接口标签");
      });
    }
  }

  function syncOperationListSelection() {
    if (!els.list) {
      return;
    }
    Array.prototype.forEach.call(els.list.querySelectorAll(".op-item"), function (button) {
      button.classList.toggle("active", button.dataset.key === state.selectedKey);
    });
  }

  function renderSelectedOperation() {
    var op = selectedOperation();
    if (!op) {
      els.tabs.innerHTML = "";
      syncOperationListSelection();
      els.content.innerHTML = renderEmptyWorkbench();
      bindEmptyWorkbenchActions();
      return;
    }
    syncOperationListSelection();
    renderWorkspaceTabFor(op);

    var params = op.parameters || [];
    var requestBody = resolveRef(op.operation.requestBody);
    var responseCount = op.operation.responses ? Object.keys(op.operation.responses).length : 0;
    var bodyCount = requestBody && requestBody.content ? Object.keys(requestBody.content).length : 0;
    var defaultRequestTab = bodyCount ? "body" : "params";
    var activeRequestTab = state.requestTabByOperation[op.key] || defaultRequestTab;
    var activeResponseTab = state.responseTabByOperation[op.key] || "body";
    var liveResponse = state.responseResultByOperation[op.key] || null;
    var hasLiveResponse = Boolean(liveResponse);
    var activeResponseView = state.responseViewByOperation[op.key] || "pretty";
    var canPreviewResponse = liveResponse && isHTMLResponse(liveResponse);
    if (activeResponseView === "preview" && !canPreviewResponse) {
      activeResponseView = "pretty";
      state.responseViewByOperation[op.key] = activeResponseView;
    }
    var showResponseToolbar = activeResponseTab === "body" && liveResponse && !liveResponse.error && Boolean(liveResponse.body);
    var responseStatus = liveResponse ? liveResponse.status || String(liveResponse.statusCode || "") : firstResponseStatus(op.operation.responses) || "200";
    var responseMeta = liveResponse ? responseRuntimeMeta(liveResponse) : responseCount + " 个示例";
    var responseLabel = liveResponse ? (liveResponse.ok ? "Success" : "Failed") : "示例";
    var liveHeaderCount = liveResponse ? responseHeaderEntries(liveResponse.headers).length : responseCount;
    var headerCount = (requestBody && requestBody.content ? 1 : 0) + ensureHeaderState(op).length;
    var requestTabs = [
      { key: "params", label: "Params", count: params.length },
      { key: "body", label: "Body", count: bodyCount },
      { key: "headers", label: "请求头", count: headerCount },
      { key: "cookies", label: "Cookies", count: 0 }
    ];
    activeRequestTab = normalizeRequestTab(activeRequestTab, requestTabs, defaultRequestTab);
    state.requestTabByOperation[op.key] = activeRequestTab;

    var html = "";
    html += '<div class="operation-workbench">';
    html += '<div class="endpoint-bar">';
    html += '<div class="endpoint-url"><button class="method endpoint-method ' + methodClass(op.method) + '" type="button" data-action="method">' + op.method.toUpperCase() + '<span class="method-caret">⌄</span></button><button class="endpoint-link" type="button" aria-label="复制接口地址" data-action="copy-url">' + icon("link") + '</button><code>' + escapeHTML(op.path) + '</code></div>';
    html += '<div class="endpoint-actions"><button class="button primary send-button" type="button" data-action="send"><span>发送</span><span class="button-icon" aria-hidden="true">' + icon("chevronDown") + '</span></button></div>';
    html += '</div>';
    html += '<div class="work-area">';
    html += '<div class="request-tabs">' + requestTabs.map(function (tab) {
      return requestTabButton(tab.key, tab.label, tab.count, activeRequestTab);
    }).join("") + '</div>';
    html += '<section class="request-pane">';
    html += '<div class="two-column"><div>';
    html += '<div class="request-tab-panel' + (activeRequestTab === "params" ? " active" : "") + '" data-request-panel="params">' + (params.length ? renderParameters(params, op) : emptyPanel("暂无 Params 参数")) + '</div>';
    html += '<div class="request-tab-panel' + (activeRequestTab === "body" ? " active" : "") + '" data-request-panel="body">' + (requestBody ? renderRequestBody(op, op.operation.requestBody) : emptyPanel("暂无请求体")) + '</div>';
    html += '<div class="request-tab-panel' + (activeRequestTab === "headers" ? " active" : "") + '" data-request-panel="headers">' + renderRequestHeaders(op, requestBody) + '</div>';
    html += '<div class="request-tab-panel' + (activeRequestTab === "cookies" ? " active" : "") + '" data-request-panel="cookies">' + emptyPanel("暂无 Cookies") + '</div>';
    html += '</div><aside class="side-panel">';
    html += '<div class="section-title">命令示例</div><pre id="curl-code">' + escapeHTML(buildCurl(op)) + '</pre>';
    html += '</aside></div>';
    html += '</section>';
    html += '<section class="response-pane">';
    if (!hasLiveResponse) {
      html += '<div class="response-placeholder">';
      html += '<div class="response-placeholder-title">返回响应</div>';
      html += '<div class="response-placeholder-panel">';
      html += '<div class="response-placeholder-icon" aria-hidden="true">' + icon("rocket") + '</div>';
      html += '<div class="response-placeholder-text">点击“发送”按钮获取返回结果</div>';
      html += '</div>';
      html += '</div>';
    } else {
      html += '<div class="response-shell">';
      html += '<div class="response-header">';
      html += '<div class="response-tabs">' +
        responseTabButton("body", "Body", 1, activeResponseTab) +
        responseTabButton("cookie", "Cookie", 0, activeResponseTab) +
        responseTabButton("header", "Header", liveHeaderCount, activeResponseTab) +
        responseTabButton("console", "控制台", 0, activeResponseTab) +
        responseTabButton("request", "实际请求", 1, activeResponseTab) +
        '</div>';
      html += '<div class="response-status-area">';
      html += '<span class="status-pill ' + (liveResponse && !liveResponse.ok ? "error" : "success") + '">' + escapeHTML(responseStatus) + '</span>';
      html += '<span class="response-meta-chip">' + escapeHTML(responseMeta) + '</span>';
      html += '<button class="response-validate" type="button" data-response-validate aria-pressed="' + (state.validateResponseByOperation[op.key] === true ? "true" : "false") + '"><span>校验响应</span><span class="response-switch"><span></span></span></button>';
      html += '<span class="response-success">' + escapeHTML(responseLabel) + (liveResponse ? " (" + escapeHTML(String(liveResponse.statusCode || "")) + ")" : "") + '</span>';
      html += '</div>';
      html += '</div>';
      if (showResponseToolbar) {
        html += '<div class="response-toolbar">';
        html += '<div class="response-view-tabs">' +
          responseViewButton("pretty", "Pretty", activeResponseView) +
          responseViewButton("raw", "Raw", activeResponseView) +
          (canPreviewResponse ? responseViewButton("preview", "Preview", activeResponseView) : "") +
          '</div>';
        html += '<button class="response-copy" type="button" data-action="copy-response" aria-label="复制响应">' + icon("copy") + '</button>';
        html += '</div>';
      }
      html += '<div id="response-output" class="response-body">' + renderResponseBody(op, activeResponseTab) + '</div>';
      html += '</div>';
    }
    html += '</section>';
    html += '</div></div>';

    els.content.innerHTML = html;
    renderWorkspaceTabFor(op);
    syncOperationListSelection();
    bindContentActions(op);
  }

  function requestTabButton(key, label, count, activeKey) {
    var countHTML = count ? ' <span class="count-dot">' + count + '</span>' : "";
    return '<button class="tab-button' + (key === activeKey ? " active" : "") + '" type="button" data-request-tab="' + key + '">' + escapeHTML(label) + countHTML + '</button>';
  }

  function normalizeRequestTab(requestTab, tabs, fallback) {
    for (var i = 0; i < tabs.length; i++) {
      if (tabs[i].key === requestTab) {
        return requestTab;
      }
    }
    return fallback;
  }

  function responseTabButton(key, label, count, activeKey) {
    var countHTML = count ? ' <span class="count-dot">' + count + '</span>' : "";
    return '<button class="tab-button' + (key === activeKey ? " active" : "") + '" type="button" data-response-tab="' + key + '">' + escapeHTML(label) + countHTML + '</button>';
  }

  function responseViewButton(key, label, activeKey) {
    return '<button class="tab-button' + (key === activeKey ? " active" : "") + '" type="button" data-response-view="' + key + '">' + escapeHTML(label) + '</button>';
  }

  function emptyPanel(text) {
    return '<div class="panel"><div class="empty-panel">' + escapeHTML(text) + '</div></div>';
  }

  function responseRuntimeMeta(response) {
    var parts = [];
    if (typeof response.durationUs === "number" && response.durationUs > 0 && response.durationUs < 1000) {
      parts.push(response.durationUs + " us");
    } else if (typeof response.durationMs === "number") {
      parts.push(response.durationMs + " ms");
    }
    if (typeof response.bytes === "number") {
      parts.push(formatBytes(response.bytes));
    }
    return parts.length ? parts.join(" / ") : "已请求";
  }

  function renderResponseBody(op, responseTab) {
    var response = state.responseResultByOperation[op.key];
    if (response) {
      if (responseTab === "cookie") {
        return renderLiveCookies(response);
      }
      if (responseTab === "header") {
        return renderLiveHeaders(response);
      }
      if (responseTab === "console") {
        return renderLiveConsole(response);
      }
      if (responseTab === "request") {
        return renderLiveRequest(response);
      }
      return renderLiveBody(op, response);
    }
    if (responseTab === "cookie") {
      return '<div class="response-empty-state"><div class="response-empty-title">暂无 Cookie</div><p>当前请求没有可展示的 Cookie 响应数据。</p></div>';
    }
    if (responseTab === "header") {
      return op.operation.responses ? renderResponses(op.operation.responses) : '<div class="response-empty-state"><div class="response-empty-title">暂无 Header</div><p>当前接口没有定义响应示例。</p></div>';
    }
    if (responseTab === "console") {
      return '<div class="response-console"><div class="response-empty-title">控制台</div><pre>' + escapeHTML(buildCurl(op)) + '</pre></div>';
    }
    if (responseTab === "request") {
      return '<div class="response-request"><div class="response-empty-title">实际请求</div><pre>' + escapeHTML(buildCurl(op)) + '</pre></div>';
    }
    return op.operation.responses ? renderResponses(op.operation.responses) : '<div class="response-empty-state"><div class="response-empty-title">点击发送获取返回结果</div><p>静态工作台不会发起真实网络请求。</p></div>';
  }

  function renderLiveBody(op, response) {
    if (response.error) {
      var title = response.proxyUnavailable ? "未连接 Go UI 代理" : "Request failed";
      return '<div class="send-result error-result"><div class="send-result-head"><span class="status-pill error">' + escapeHTML(response.status || "Error") + '</span><span>' + escapeHTML(title) + '</span></div><pre>' + escapeHTML(response.error) + '</pre></div>';
    }
    if (!response.body) {
      return '<div class="response-empty-state"><div class="response-empty-title">响应体为空</div><p>状态、耗时和响应头已经更新。</p></div>';
    }
    var view = state.responseViewByOperation[op.key] || "pretty";
    if (view === "preview" && isHTMLResponse(response)) {
      return '<iframe class="response-preview-frame" title="响应预览" sandbox srcdoc="' + escapeAttribute(response.body) + '"></iframe>';
    }
    var body = view === "raw" ? response.body : prettyBody(response.body, response.contentType || headerValue(response.headers, "Content-Type"));
    return '<pre class="response-code">' + escapeHTML(body) + '</pre>';
  }

  function renderLiveHeaders(response) {
    var rows = responseHeaderEntries(response.headers).map(function (header) {
      return "<tr><td>" + escapeHTML(header.name) + "</td><td>" + escapeHTML(header.values.join(", ")) + "</td></tr>";
    }).join("");
    if (!rows) {
      return '<div class="response-empty-state"><div class="response-empty-title">暂无 Header</div><p>本次响应没有返回可展示的 Header。</p></div>';
    }
    return '<div class="panel response-table-panel"><table class="table response-table response-headers-table"><colgroup><col class="response-header-key-col" /><col /></colgroup><thead><tr><th>Key</th><th>Value</th></tr></thead><tbody>' + rows + '</tbody></table></div>';
  }

  function renderLiveCookies(response) {
    var rows = responseHeaderEntries(response.headers).filter(function (header) {
      return header.name.toLowerCase() === "set-cookie";
    }).map(function (header) {
      return header.values.map(function (value) {
        return "<tr><td>Set-Cookie</td><td>" + escapeHTML(value) + "</td></tr>";
      }).join("");
    }).join("");
    if (!rows) {
      return '<div class="response-empty-state"><div class="response-empty-title">暂无 Cookie</div><p>本次响应没有 Set-Cookie 响应头。</p></div>';
    }
    return '<div class="panel response-table-panel"><table class="table response-table"><thead><tr><th>Cookie</th><th>Value</th></tr></thead><tbody>' + rows + '</tbody></table></div>';
  }

  function renderLiveConsole(response) {
    return '<div class="response-console"><div class="response-empty-title">Console</div><pre>' + escapeHTML(JSON.stringify({
      status: response.status,
      durationMs: response.durationMs,
      durationUs: response.durationUs || 0,
      bytes: response.bytes,
      error: response.error || ""
    }, null, 2)) + '</pre></div>';
  }

  function renderLiveRequest(response) {
    var request = response.request || {};
    return '<div class="response-request"><div class="response-empty-title">Actual Request</div><pre>' + escapeHTML(JSON.stringify({
      method: request.method || response.method,
      url: request.url || response.url,
      headers: request.headers || [],
      body: request.body || ""
    }, null, 2)) + '</pre></div>';
  }

  function prettyBody(body, contentType) {
    var maybeJSON = (contentType || "").toLowerCase().indexOf("json") >= 0 || /^[\s\n]*[\[{]/.test(body);
    if (!maybeJSON) {
      return body;
    }
    try {
      return JSON.stringify(JSON.parse(body), null, 2);
    } catch (err) {
      return body;
    }
  }

  function isHTMLResponse(response) {
    return (response.contentType || headerValue(response.headers, "Content-Type") || "").toLowerCase().indexOf("html") >= 0;
  }

  function responseHeaderEntries(headers) {
    if (!headers) {
      return [];
    }
    if (Array.isArray(headers)) {
      return headers.map(function (header) {
        return {
          name: header.name || "",
          values: header.values || (header.value ? [header.value] : [])
        };
      }).filter(function (header) {
        return header.name;
      });
    }
    return Object.keys(headers).sort().map(function (name) {
      var values = headers[name];
      if (!Array.isArray(values)) {
        values = values == null ? [] : [String(values)];
      }
      return { name: name, values: values.map(String) };
    });
  }

  function headerValue(headers, key) {
    var normalized = key.toLowerCase();
    var entries = responseHeaderEntries(headers);
    for (var i = 0; i < entries.length; i++) {
      if (entries[i].name.toLowerCase() === normalized) {
        return entries[i].values.join(", ");
      }
    }
    return "";
  }

  function formatBytes(bytes) {
    if (!bytes) {
      return "0 B";
    }
    if (bytes < 1024) {
      return bytes + " B";
    }
    if (bytes < 1024 * 1024) {
      return (bytes / 1024).toFixed(1) + " KB";
    }
    return (bytes / 1024 / 1024).toFixed(1) + " MB";
  }

  function renderParameters(parameters, op) {
    var rows = parameters.map(function (ref) {
      var parameter = resolveRef(ref) || {};
      var schema = parameter.schema ? resolveSchema(parameter.schema) : null;
      var value = op ? parameterCurrentValue(op, parameter, schema) : "";
      return "<tr>" +
        '<td class="check-cell"><span class="row-check"></span></td>' +
        "<td>" + escapeHTML(parameter.name || "") + "</td>" +
        "<td>" + renderParameterValueControl(parameter, schema, value) + "</td>" +
        '<td class="type-cell">' + escapeHTML(schemaLabel(schema || parameter.schema)) + (parameter.required ? ' <span class="required-star">*</span>' : "") + "</td>" +
        "<td>" + escapeHTML(parameter.description || "") + "</td>" +
        "</tr>";
    }).join("");

    return '<div class="section-title">参数</div><div class="panel"><table class="table"><thead><tr><th></th><th>参数名</th><th>参数值</th><th>类型</th><th>说明</th></tr></thead><tbody>' +
	      rows + '</tbody></table></div>';
	  }

  function renderParameterValueControl(parameter, schema, value) {
    var name = parameter.name || "";
    var location = parameter.in || "";
    if (schema && Array.isArray(schema.enum) && schema.enum.length) {
      var options = schema.enum.map(function (item) {
        var optionValue = String(item);
        return '<option value="' + escapeAttribute(optionValue) + '"' + (String(value) === optionValue ? " selected" : "") + '>' + escapeHTML(optionValue) + '</option>';
      }).join("");
      return '<select class="table-input value-input" data-param-name="' + escapeAttribute(name) + '" data-param-in="' + escapeAttribute(location) + '">' + options + '</select>';
    }
    return '<input class="table-input value-input" type="text" data-param-name="' + escapeAttribute(name) + '" data-param-in="' + escapeAttribute(location) + '" value="' + escapeAttribute(value) + '" placeholder="参考值" />';
  }

  function renderRequestBody(op, bodyRef) {
    var body = resolveRef(bodyRef);
    if (!body || !body.content) {
      return "";
    }
    var contentTypes = Object.keys(body.content);
    var activeContentType = inferBodyFormat(contentTypes);
    var media = body.content[activeContentType] || body.content[contentTypes[0]];
    var table = media ? renderSchemaTable(op, media.schema, bodyExampleMap(op, activeContentType, media.example || body.example || {}), activeContentType) : "";
    var parts = table || (media ? '<div class="schema-node"><div class="schema-title"><span class="schema-name">' + escapeHTML(activeContentType || contentTypes[0]) + '</span><span class="schema-type">' + (body.required ? "必填" : "可选") + '</span></div>' +
      renderSchema(media.schema, "请求体", 0) + '</div>' : "");
    return '<div class="section-title">请求体</div><div class="panel"><div class="body-type-bar"><span>Content-Type</span><code>' + escapeHTML(activeContentType || contentTypes[0] || "") + '</code></div>' + parts + '</div>';
  }

  function inferBodyFormat(contentTypes) {
    var normalized = (contentTypes || []).map(function (item) {
      return String(item || "").toLowerCase();
    });
    var matchers = [
      "multipart/form-data",
      "application/x-www-form-urlencoded",
      "application/json",
      "application/xml",
      "text/xml",
      "text/plain",
      "application/octet-stream",
      "application/graphql",
      "application/msgpack"
    ];
    for (var i = 0; i < matchers.length; i++) {
      for (var j = 0; j < normalized.length; j++) {
        if (normalized[j].indexOf(matchers[i]) >= 0) {
          return normalized[j];
        }
      }
    }
    return normalized[0] || "";
  }

  function renderSchemaTable(op, schemaRef, exampleMap, contentType) {
    var schema = resolveSchema(schemaRef);
    if (!schema || !schema.properties) {
      return "";
    }
    var required = new Set(schema.required || []);
    var rows = Object.keys(schema.properties).map(function (name) {
      var child = resolveSchema(schema.properties[name]) || schema.properties[name];
      var exampleValue = exampleMap && Object.prototype.hasOwnProperty.call(exampleMap, name) ? exampleMap[name] : child.example;
      return "<tr>" +
        '<td class="check-cell"><span class="row-check"></span></td>' +
        "<td>" + escapeHTML(name) + "</td>" +
        "<td>" + renderBodyValueControl(op, name, child, contentType, exampleValue !== undefined ? exampleValue : child.default) + "</td>" +
        '<td class="type-cell">' + escapeHTML(schemaTypeLabel(child)) + (required.has(name) ? ' <span class="required-star">*</span>' : "") + "</td>" +
        "<td>" + escapeHTML(child && child.description ? child.description : "") + "</td>" +
        "</tr>";
    }).join("");
    return '<table class="table"><thead><tr><th></th><th>参数名</th><th>参数值</th><th>类型</th><th>说明</th></tr></thead><tbody>' + rows + '</tbody></table>';
  }

  function renderBodyValueControl(op, name, schema, contentType, value) {
    if (isFileSchema(schema)) {
      var file = bodyFileValue(op, contentType, name);
      return '<label class="file-picker">' +
        '<input type="file" data-body-file-field="' + escapeAttribute(name) + '" data-body-example-content-type="' + escapeAttribute(contentType || "") + '" />' +
        '<span class="file-picker-button">' + icon("upload") + '<span>' + (file ? "重新选择" : "选择文件") + '</span></span>' +
        '<span class="file-picker-name">' + escapeHTML(file ? file.name : "未选择文件") + '</span>' +
        '</label>';
    }
    if (schema && Array.isArray(schema.enum) && schema.enum.length) {
      var current = exampleText(value);
      var options = schema.enum.map(function (item) {
        var optionValue = String(item);
        return '<option value="' + escapeAttribute(optionValue) + '"' + (String(current) === optionValue ? " selected" : "") + '>' + escapeHTML(optionValue) + '</option>';
      }).join("");
      return '<select class="table-input body-example-input" data-body-example-field="' + escapeAttribute(name) + '" data-body-example-content-type="' + escapeAttribute(contentType || "") + '">' + options + '</select>';
    }
    return '<input class="table-input body-example-input" type="text" data-body-example-field="' + escapeAttribute(name) + '" data-body-example-content-type="' + escapeAttribute(contentType || "") + '" value="' + escapeAttribute(exampleText(value)) + '" placeholder="参考值" />';
  }

  function bodyExampleMap(op, contentType, baseExample) {
    var overrides = (state.bodyExamplesByOperation[op.key] && state.bodyExamplesByOperation[op.key][contentType]) || {};
    return Object.assign({}, baseExample || {}, overrides);
  }

  function setBodyExample(op, contentType, field, value) {
    if (!state.bodyExamplesByOperation[op.key]) {
      state.bodyExamplesByOperation[op.key] = {};
    }
    if (!state.bodyExamplesByOperation[op.key][contentType]) {
      state.bodyExamplesByOperation[op.key][contentType] = {};
    }
    state.bodyExamplesByOperation[op.key][contentType][field] = value;
  }

  function bodyFileMap(op, contentType) {
    if (!state.bodyFilesByOperation[op.key]) {
      state.bodyFilesByOperation[op.key] = {};
    }
    if (!state.bodyFilesByOperation[op.key][contentType]) {
      state.bodyFilesByOperation[op.key][contentType] = {};
    }
    return state.bodyFilesByOperation[op.key][contentType];
  }

  function setBodyFile(op, contentType, field, file) {
    if (!field) {
      return;
    }
    var files = bodyFileMap(op, contentType);
    if (file) {
      files[field] = file;
    } else {
      delete files[field];
    }
  }

  function bodyFileValue(op, contentType, field) {
    return (((state.bodyFilesByOperation[op.key] || {})[contentType] || {})[field]) || null;
  }

  function parameterValueMap(op) {
    if (!state.parameterValuesByOperation[op.key]) {
      state.parameterValuesByOperation[op.key] = {};
    }
    return state.parameterValuesByOperation[op.key];
  }

  function parameterValueKey(name, location) {
    return (location || "") + ":" + (name || "");
  }

  function setParameterValue(op, name, location, value) {
    if (!name) {
      return;
    }
    parameterValueMap(op)[parameterValueKey(name, location)] = value;
  }

  function parameterCurrentValue(op, parameter, schema) {
    var values = parameterValueMap(op);
    var key = parameterValueKey(parameter.name, parameter.in);
    if (Object.prototype.hasOwnProperty.call(values, key)) {
      return values[key];
    }
    return exampleText(parameterExampleValue(parameter, schema));
  }

  function parameterExampleValue(parameter, schema) {
    if (parameter && parameter.example !== undefined) {
      return parameter.example;
    }
    if (schema && schema.default !== undefined) {
      return schema.default;
    }
    if (schema && schema.enum && schema.enum.length) {
      return schema.enum[0];
    }
    if (schema && (schema.type === "integer" || schema.type === "number") && schema.minimum !== undefined) {
      return schema.minimum;
    }
    if (schema && schema.type === "boolean") {
      return false;
    }
    return "";
  }

  function renderRequestHeaders(op, body) {
    var rows = [];
    var contentTypes = body && body.content ? Object.keys(body.content) : [];
    var activeContentType = inferBodyFormat(contentTypes);
    var hasBlankHeaderKey = false;
    if (activeContentType) {
      rows.push(headerRow({
        key: "Content-Type",
        value: activeContentType,
        type: "string",
        description: "由请求体自动识别",
        enabled: true,
        locked: true
      }, -1));
    }

    ensureHeaderState(op).forEach(function (header, index) {
      if (!hasBlankHeaderKey && !header.locked && !(header.key || "").trim()) {
        hasBlankHeaderKey = true;
      }
      rows.push(headerRow(header, index));
    });

    var suggestionsOpen = state.headerSuggestionsOpenByOperation[op.key] === true || hasBlankHeaderKey;
    if (suggestionsOpen) {
      rows.push(headerSuggestionsRow(op, Boolean(activeContentType)));
    }
    return '<div class="panel headers-panel request-headers-panel"><div class="request-header-grid" role="table" aria-label="请求 Header">' +
      '<div class="request-header-head" role="row"><span></span><span role="columnheader">参数名</span><span role="columnheader">类型</span><span role="columnheader">默认值</span><span role="columnheader">默认启用</span><span role="columnheader">说明</span><span></span></div>' +
      rows.join("") +
      '<button class="env-empty-row request-header-add" type="button" data-add-header-button>添加参数</button>' +
      '</div></div>';
  }

  function setHeaderSuggestionsOpen(op, open) {
    state.headerSuggestionsOpenByOperation[op.key] = open;
  }

  function addBlankRequestHeader(op) {
    var headers = ensureHeaderState(op);
    for (var i = 0; i < headers.length; i++) {
      if (!headers[i].locked && !(headers[i].key || "").trim()) {
        setHeaderSuggestionsOpen(op, true);
        return;
      }
    }
    headers.push({ key: "", value: "", type: "string", description: "", enabled: true, locked: false });
    setHeaderSuggestionsOpen(op, true);
  }

  function openHeaderSuggestions(op) {
    if (state.headerSuggestionsOpenByOperation[op.key] !== true) {
      setHeaderSuggestionsOpen(op, true);
      renderSelectedOperation();
    }
  }

  function ensureHeaderState(op) {
    if (!state.headersByOperation[op.key]) {
      state.headersByOperation[op.key] = [];
    }
    return state.headersByOperation[op.key];
  }

  function headerDescription(key) {
    var descriptions = {
      Accept: "客户端可接受的响应格式。",
      "Accept-Charset": "可接受的字符集。",
      "Accept-Encoding": "可接受的压缩编码。",
      "Accept-Language": "可接受的语言。",
      Authorization: "访问令牌，例如 Bearer token。",
      "Cache-Control": "缓存控制策略。",
      "Content-MD5": "请求体摘要。",
      Cookie: "Cookie 信息。",
      "If-Match": "条件请求标记。",
      "If-Modified-Since": "条件请求时间。",
      "If-None-Match": "条件请求标记。",
      Origin: "请求来源。",
      Referer: "来源页面。",
      "User-Agent": "客户端标识。",
      "X-API-Key": "自定义 API 密钥。",
      "X-Request-ID": "请求追踪 ID。"
    };
    return descriptions[key] || "常用请求头。";
  }

  function headerRow(header, index) {
    var locked = header.locked ? " readonly" : "";
    var disabled = header.enabled === false ? " unchecked" : "";
    var typeLocked = header.locked ? " disabled" : "";
    var indexAttr = index >= 0 ? ' data-header-index="' + index + '"' : "";
    return '<div class="request-header-row' + (header.locked ? " locked" : "") + '" role="row"' + indexAttr + '>' +
      '<span class="env-drag-handle" aria-hidden="true">' + icon("grip") + '</span>' +
      '<input class="env-header-input request-header-name-input" data-header-field="key" value="' + escapeAttribute(header.key || "") + '" placeholder="Header 名称"' + locked + ' />' +
      '<select class="env-header-select" data-header-field="type"' + typeLocked + '>' + headerTypeOptions(header.type) + '</select>' +
      '<input class="env-header-input" data-header-field="value" value="' + escapeAttribute(header.value || "") + '"' + locked + ' placeholder="Header 默认值" />' +
      '<button class="env-header-switch' + disabled + (header.locked ? " locked" : "") + '" type="button" data-header-enabled aria-pressed="' + (header.enabled === false ? "false" : "true") + '"><span></span></button>' +
      '<input class="env-header-input" data-header-field="description" value="' + escapeAttribute(header.description || "") + '"' + locked + ' placeholder="说明" />' +
      (header.locked ? '<span></span>' : '<button class="env-row-remove" type="button" data-header-remove aria-label="移除 Header">' + icon("minus") + '</button>') +
      '</div>';
  }

  function headerTypeOptions(type) {
    var value = (type || "string").toString();
    var options = [];
    var seen = {};
    if (HEADER_TYPES.indexOf(value) < 0) {
      options.push(value);
      seen[value] = true;
    }
    HEADER_TYPES.forEach(function (item) {
      if (!seen[item]) {
        options.push(item);
      }
    });
    return options.map(function (item) {
      return '<option value="' + escapeAttribute(item) + '"' + (item === value ? ' selected' : "") + '>' + escapeHTML(item) + '</option>';
    }).join("");
  }

  function headerSuggestionsRow(op, hasContentType) {
    var used = {};
    if (hasContentType) {
      used["content-type"] = true;
    }
    ensureHeaderState(op).forEach(function (header) {
      if (header.key) {
        used[header.key.toLowerCase()] = true;
      }
    });
    var suggestions = COMMON_HEADERS.filter(function (name) {
      return !used[name.toLowerCase()];
    });
    if (!suggestions.length) {
      return '<div class="header-suggestions-row"><div class="header-suggestion-menu"><span class="header-suggestion-empty">暂无更多常用 Header</span></div></div>';
    }
    return '<div class="header-suggestions-row"><div class="header-suggestion-menu">' + suggestions.map(function (name) {
      return '<button class="header-suggestion" type="button" data-header-suggestion="' + escapeAttribute(name) + '">' + escapeHTML(name) + '</button>';
    }).join("") + '</div></div>';
  }

  function exampleText(value) {
    if (value == null) {
      return "";
    }
    if (typeof value === "string") {
      return value;
    }
    if (typeof value === "number" || typeof value === "boolean") {
      return String(value);
    }
    return JSON.stringify(value);
  }

  function renderResponses(responses) {
    var parts = Object.keys(responses).sort().map(function (status) {
      var response = resolveRef(responses[status]) || {};
      var content = response.content || {};
      var media = Object.keys(content).map(function (contentType) {
        return '<div class="schema-node"><div class="schema-title"><span class="schema-name">' + escapeHTML(contentType) + '</span></div>' +
          renderSchema(content[contentType].schema, "响应体", 0) + '</div>';
      }).join("");
      return '<details class="schema-node" open><summary><span class="schema-title"><span class="status-pill">' + escapeHTML(status) + '</span><span>' + escapeHTML(response.description || "") + '</span></span></summary>' +
        '<div class="schema-children">' + (media || '<div class="schema-node muted">无响应体</div>') + '</div></details>';
    }).join("");
    return '<div class="panel">' + parts + '</div>';
  }

  function renderSchema(schemaRef, name, depth) {
    if (!schemaRef) {
      return '<div class="schema-desc">无结构定义</div>';
    }
    var resolved = resolveSchema(schemaRef);
    if (!resolved) {
      return '<div class="schema-desc">' + escapeHTML(schemaRef.$ref || "未知结构") + '</div>';
    }
    var label = schemaLabel(resolved);
    var properties = resolved.properties || {};
    var propertyNames = Object.keys(properties);
    var required = new Set(resolved.required || []);
    var open = depth < 2 ? " open" : "";
    var html = '<details class="schema-node"' + open + '><summary><span class="schema-title"><span class="schema-name">' + escapeHTML(name) + '</span><span class="schema-type">' + escapeHTML(label) + '</span></span></summary>';
    if (resolved.description) {
      html += '<p class="schema-desc">' + escapeHTML(resolved.description) + '</p>';
    }
    if (propertyNames.length) {
      html += '<div class="schema-children">';
      propertyNames.forEach(function (key) {
        var childName = required.has(key) ? key + " *" : key;
        html += renderSchema(properties[key], childName, depth + 1);
      });
      html += '</div>';
    } else if (resolved.items) {
      html += '<div class="schema-children">' + renderSchema(resolved.items, "array item", depth + 1) + '</div>';
    } else if (resolved.additionalProperties && typeof resolved.additionalProperties === "object") {
      html += '<div class="schema-children">' + renderSchema(resolved.additionalProperties, "additionalProperties", depth + 1) + '</div>';
    }
    html += '</details>';
    return html;
  }

  function bindContentActions(op) {
    Array.prototype.forEach.call(els.content.querySelectorAll("[data-request-tab]"), function (button) {
      button.addEventListener("click", function () {
        var key = button.dataset.requestTab || "params";
        state.requestTabByOperation[op.key] = key;
        setActiveSibling(button);
        Array.prototype.forEach.call(els.content.querySelectorAll("[data-request-panel]"), function (panel) {
          panel.classList.toggle("active", panel.dataset.requestPanel === key);
        });
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-response-tab]"), function (button) {
      button.addEventListener("click", function () {
        var key = button.dataset.responseTab || "body";
        state.responseTabByOperation[op.key] = key;
        renderSelectedOperation();
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-response-view]"), function (button) {
      button.addEventListener("click", function () {
        state.responseViewByOperation[op.key] = button.dataset.responseView || "pretty";
        renderSelectedOperation();
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-response-validate]"), function (button) {
      button.addEventListener("click", function () {
        var next = button.getAttribute("aria-pressed") !== "true";
        button.setAttribute("aria-pressed", next ? "true" : "false");
        state.validateResponseByOperation[op.key] = next;
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-header-enabled]"), function (toggle) {
      toggle.addEventListener("click", function () {
        if (toggle.classList.contains("locked")) {
          showToast("Content-Type 为自动识别项，不能取消");
          return;
        }
        toggle.classList.toggle("unchecked");
        var row = toggle.closest ? toggle.closest("[data-header-index]") : null;
        if (row) {
          var headers = ensureHeaderState(op);
          var index = Number(row.dataset.headerIndex);
          if (headers[index]) {
            headers[index].enabled = !toggle.classList.contains("unchecked");
            toggle.setAttribute("aria-pressed", headers[index].enabled ? "true" : "false");
          }
        }
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-header-remove]"), function (button) {
      button.addEventListener("click", function () {
        var row = button.closest("[data-header-index]");
        if (!row) {
          return;
        }
        var headers = ensureHeaderState(op);
        headers.splice(Number(row.dataset.headerIndex), 1);
        renderSelectedOperation();
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-header-field]"), function (input) {
      if (input.dataset.headerField === "key") {
        input.addEventListener("click", function () {
          if (!input.value.trim()) {
            openHeaderSuggestions(op);
          }
        });
        input.addEventListener("focus", function () {
          if (!input.value.trim()) {
            openHeaderSuggestions(op);
          }
        });
      }
      input.addEventListener("input", function () {
        var row = input.closest("[data-header-index]");
        if (!row) {
          return;
        }
        var headers = ensureHeaderState(op);
        var index = Number(row.dataset.headerIndex);
        var field = input.dataset.headerField;
        if (headers[index] && field) {
          headers[index][field] = input.value;
          if (field === "key" && !input.value.trim()) {
            openHeaderSuggestions(op);
          }
        }
      });
      if (input.tagName === "SELECT") {
        input.addEventListener("change", function () {
          var row = input.closest("[data-header-index]");
          if (!row) {
            return;
          }
          var headers = ensureHeaderState(op);
          var index = Number(row.dataset.headerIndex);
          var field = input.dataset.headerField;
          if (headers[index] && field) {
            headers[index][field] = input.value;
          }
        });
      }
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-param-name]"), function (input) {
      var syncParameter = function () {
        setParameterValue(op, input.dataset.paramName || "", input.dataset.paramIn || "", input.value);
      };
      input.addEventListener("input", syncParameter);
      input.addEventListener("change", syncParameter);
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-body-example-field]"), function (input) {
      var syncBodyExample = function () {
        var field = input.dataset.bodyExampleField;
        var contentType = input.dataset.bodyExampleContentType || "";
        if (!field) {
          return;
        }
        setBodyExample(op, contentType, field, input.value);
      };
      input.addEventListener("input", syncBodyExample);
      input.addEventListener("change", syncBodyExample);
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-body-file-field]"), function (input) {
      input.addEventListener("change", function () {
        var field = input.dataset.bodyFileField;
        var contentType = input.dataset.bodyExampleContentType || "";
        setBodyFile(op, contentType, field, input.files && input.files.length ? input.files[0] : null);
        renderSelectedOperation();
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-add-header], [data-add-header-button]"), function (button) {
      button.addEventListener("click", function () {
        addBlankRequestHeader(op);
        renderSelectedOperation();
      });
    });

    Array.prototype.forEach.call(els.content.querySelectorAll("[data-header-suggestion]"), function (button) {
      button.addEventListener("click", function () {
        var key = button.dataset.headerSuggestion || "";
        var headers = ensureHeaderState(op);
        var target = null;
        for (var i = 0; i < headers.length; i++) {
          if (!headers[i].locked && !(headers[i].key || "").trim()) {
            target = headers[i];
            break;
          }
        }
        if (target) {
          target.key = key;
          target.type = "string";
          target.description = headerDescription(key);
          target.enabled = true;
        } else {
          headers.push({
            key: key,
            value: "",
            type: "string",
            description: headerDescription(key),
            enabled: true,
            locked: false
          });
        }
        setHeaderSuggestionsOpen(op, false);
        renderSelectedOperation();
        showToast("已添加 " + key);
      });
    });

    bindAction("copy-url", function () {
      copyText(buildURL(op)).then(function () {
        showToast("接口地址已复制");
      });
    });

    bindAction("copy-response", function () {
      var response = state.responseResultByOperation[op.key];
      copyText(response && response.body ? response.body : "").then(function () {
        showToast("响应已复制");
      });
    });

    bindAction("method", function () {
      showToast("请求方法来自 OpenAPI 文档，暂不在预览中修改");
    });

    bindAction("send", function (button) {
      sendOperation(op, button);
    });
  }

  function bindAction(action, handler) {
    var button = els.content.querySelector('[data-action="' + action + '"]');
    if (!button) {
      return;
    }
    button.addEventListener("click", function () {
      handler(button);
    });
  }

  function setActiveSibling(button) {
    Array.prototype.forEach.call(button.parentNode.querySelectorAll(".tab-button"), function (item) {
      item.classList.toggle("active", item === button);
    });
  }

  function flashButton(button, text) {
    var label = button.querySelector("span:first-child") || button;
    var previous = label.textContent;
    label.textContent = text;
    window.setTimeout(function () {
      label.textContent = previous;
    }, 1200);
  }

  async function sendOperation(op, button) {
    flashButton(button, "发送中");
    state.responseTabByOperation[op.key] = "body";
    state.responseViewByOperation[op.key] = "pretty";
    var payload;
    try {
      payload = await buildProxyPayload(op);
    } catch (err) {
      state.responseResultByOperation[op.key] = requestBuildErrorResponse(op, err);
      renderSelectedOperation();
      showToast("请求构建失败");
      return;
    }
    if (isLocalFilePage()) {
      state.responseResultByOperation[op.key] = previewResponse(op, payload);
      renderSelectedOperation();
      showToast("静态模式已生成预览响应");
      return;
    }
    try {
      var response = await fetch(proxyURL(), {
        method: "POST",
        headers: { "Content-Type": "application/json", "Accept": "application/json" },
        body: JSON.stringify(payload)
      });
      if ((response.headers.get("Content-Type") || "").toLowerCase().indexOf("json") < 0) {
        throw new Error("proxy returned non-json response");
      }
      var data = await response.json();
      data.request = data.request || payload;
      if (!response.ok && !data.error) {
        data.error = "HTTP " + response.status;
      }
      state.responseResultByOperation[op.key] = data;
      renderSelectedOperation();
      if (data.error) {
        showToast("请求失败");
      }
    } catch (err) {
      state.responseResultByOperation[op.key] = previewResponse(op, payload, err);
      renderSelectedOperation();
      showToast("未连接 Go UI 代理，已显示预览");
    }
  }

  function requestBuildErrorResponse(op, err) {
    var body = err && err.message ? err.message : "请求构建失败";
    return {
      ok: false,
      method: op.method.toUpperCase(),
      url: buildRequestURL(op),
      statusCode: 0,
      status: "请求构建失败",
      durationMs: 0,
      durationUs: 0,
      bytes: body.length,
      contentType: "text/plain; charset=utf-8",
      headers: { "Content-Type": ["text/plain; charset=utf-8"] },
      body: body,
      request: { method: op.method.toUpperCase(), url: buildRequestURL(op), headers: [] },
      error: body
    };
  }

  function proxyURL() {
    return new URL("./__openapi-ui/request", window.location.href).toString();
  }

  function previewResponse(op, payload, err) {
    var status = Number(firstResponseStatus(op.operation.responses)) || 200;
    var proxyUnavailableText = "未连接 Go UI 代理，无法发起真实请求。\n\n请通过 Go 服务启动 OpenAPI UI，或确认 /__openapi-ui/request 路由可访问。";
    var body = err ? proxyUnavailableText : "静态模式不会发起真实网络请求。\n\n" + buildCurl(op);
    return {
      ok: !err,
      method: payload.method,
      url: payload.url,
      statusCode: err ? 0 : status,
      status: err ? "未连接 Go UI 代理" : String(status),
      durationMs: 0,
      durationUs: 0,
      bytes: body.length,
      contentType: "text/plain; charset=utf-8",
      headers: { "Content-Type": ["text/plain; charset=utf-8"] },
      body: body,
      request: payload,
      error: err ? proxyUnavailableText : "",
      proxyUnavailable: Boolean(err)
    };
  }

  async function buildProxyPayload(op) {
    var bodyInfo = await buildRequestBody(op);
    return {
      method: op.method.toUpperCase(),
      url: buildRequestURL(op),
      headers: buildRequestHeaders(op, bodyInfo.contentType),
      body: bodyInfo.body,
      formFields: bodyInfo.formFields || [],
      files: bodyInfo.files || []
    };
  }

  function buildRequestURL(op) {
    var path = op.path.replace(/\{([^}]+)\}/g, function (_, name) {
      var parameter = findParameter(op, name, "path");
      if (!parameter) {
        return encodeURIComponent(name);
      }
      var schema = parameter.schema ? resolveSchema(parameter.schema) : null;
      return encodeURIComponent(parameterCurrentValue(op, parameter, schema));
    });
    var query = buildQueryString(op);
    var base = state.serverURL || "";
    var url = base ? base.replace(/\/$/, "") + path : path;
    if (query) {
      url += (url.indexOf("?") >= 0 ? "&" : "?") + query;
    }
    return url;
  }

  function buildQueryString(op) {
    var pairs = [];
    (op.parameters || []).forEach(function (ref) {
      var parameter = resolveRef(ref) || {};
      if (parameter.in !== "query") {
        return;
      }
      var schema = parameter.schema ? resolveSchema(parameter.schema) : null;
      var value = parameterCurrentValue(op, parameter, schema);
      if (value === "" || value == null) {
        return;
      }
      pairs.push(encodeURIComponent(parameter.name) + "=" + encodeURIComponent(value));
    });
    return pairs.join("&");
  }

  function findParameter(op, name, location) {
    for (var i = 0; i < (op.parameters || []).length; i++) {
      var parameter = resolveRef(op.parameters[i]) || {};
      if (parameter.name === name && (!location || parameter.in === location)) {
        return parameter;
      }
    }
    return null;
  }

  function buildRequestHeaders(op, contentType) {
    var headers = [];
    var byName = {};
    function setHeader(name, value) {
      var key = (name || "").trim();
      if (!key) {
        return;
      }
      var normalized = key.toLowerCase();
      if (byName[normalized] != null) {
        headers[byName[normalized]] = { name: key, value: value || "" };
        return;
      }
      byName[normalized] = headers.length;
      headers.push({ name: key, value: value || "" });
    }
    (state.globalHeaders || []).forEach(function (header) {
      if (header.enabled === false || !(header.key || "").trim()) {
        return;
      }
      if (header.key.toLowerCase() === "content-type" && contentType) {
        return;
      }
      setHeader(header.key, header.value || "");
    });
    if (contentType) {
      setHeader("Content-Type", contentType);
    }
    ensureHeaderState(op).forEach(function (header) {
      if (header.enabled === false || !(header.key || "").trim()) {
        return;
      }
      if (header.key.toLowerCase() === "content-type" && contentType) {
        return;
      }
      setHeader(header.key, header.value || "");
    });
    return headers;
  }

  function requestBodyContentType(op) {
    var requestBody = resolveRef(op.operation.requestBody);
    if (!requestBody || !requestBody.content) {
      return "";
    }
    return inferBodyFormat(Object.keys(requestBody.content));
  }

  async function buildRequestBody(op) {
    var requestBody = resolveRef(op.operation.requestBody);
    if (!requestBody || !requestBody.content) {
      return { contentType: "", body: "" };
    }
    var contentTypes = Object.keys(requestBody.content);
    var contentType = inferBodyFormat(contentTypes);
    var media = requestBody.content[contentType] || requestBody.content[contentTypes[0]] || {};
    var example = bodyExampleMap(op, contentType, media.example || requestBody.example || {});
    if (contentType.indexOf("multipart/form-data") >= 0) {
      return buildMultipartBody(op, contentType, media.schema, example);
    }
    if (contentType.indexOf("application/x-www-form-urlencoded") >= 0) {
      return { contentType: contentType, body: formEncode(example) };
    }
    if (contentType.indexOf("json") >= 0) {
      return { contentType: contentType, body: JSON.stringify(example || {}, null, 2) };
    }
    if (contentType.indexOf("text/") === 0) {
      return { contentType: contentType, body: typeof example === "string" ? example : Object.keys(example || {}).map(function (key) { return example[key]; }).join("\n") };
    }
    return { contentType: contentType, body: JSON.stringify(example || {}) };
  }

  async function buildMultipartBody(op, contentType, schemaRef, example) {
    var schema = resolveSchema(schemaRef);
    var fields = [];
    var files = [];
    var properties = schema && schema.properties ? schema.properties : {};
    var names = Object.keys(properties);
    for (var i = 0; i < names.length; i++) {
      var name = names[i];
      var child = resolveSchema(properties[name]) || properties[name];
      if (isFileSchema(child)) {
        var file = bodyFileValue(op, contentType, name);
        if (file) {
          files.push(await filePayload(name, file));
        }
        continue;
      }
      var value = example && Object.prototype.hasOwnProperty.call(example, name) ? example[name] : child.example;
      if (value !== undefined && value !== null) {
        fields.push({ name: name, value: String(value) });
      }
    }
    Object.keys(example || {}).forEach(function (name) {
      if (Object.prototype.hasOwnProperty.call(properties, name)) {
        return;
      }
      fields.push({ name: name, value: String(example[name]) });
    });
    return { contentType: contentType, body: "", formFields: fields, files: files };
  }

  async function filePayload(name, file) {
    if (file.size > MAX_UPLOAD_FILE_BYTES) {
      throw new Error("文件超过上传限制：" + formatBytes(file.size) + " / " + formatBytes(MAX_UPLOAD_FILE_BYTES));
    }
    return {
      name: name,
      filename: file.name,
      contentType: file.type || "application/octet-stream",
      dataBase64: await readFileBase64(file)
    };
  }

  function formatBytes(value) {
    if (value >= 1024 * 1024) {
      return (value / 1024 / 1024).toFixed(1) + " MB";
    }
    if (value >= 1024) {
      return (value / 1024).toFixed(1) + " KB";
    }
    return String(value) + " B";
  }

  function readFileBase64(file) {
    return new Promise(function (resolve, reject) {
      var reader = new FileReader();
      reader.onload = function () {
        var value = String(reader.result || "");
        resolve(value.indexOf(",") >= 0 ? value.split(",").pop() : value);
      };
      reader.onerror = function () {
        reject(reader.error || new Error("read file failed"));
      };
      reader.readAsDataURL(file);
    });
  }

  function formEncode(values) {
    var params = new URLSearchParams();
    Object.keys(values || {}).forEach(function (key) {
      var value = values[key];
      if (value == null) {
        value = "";
      }
      if (typeof value === "object") {
        value = JSON.stringify(value);
      }
      params.append(key, String(value));
    });
    return params.toString();
  }

  function firstResponseStatus(responses) {
    if (!responses) {
      return "";
    }
    return Object.keys(responses).sort()[0] || "";
  }

  function buildURL(op) {
    return buildRequestURL(op);
  }

  function selectedOperation() {
    return state.operations.find(function (item) { return item.key === state.selectedKey; });
  }

  function collectOperations(spec) {
    var operations = [];
    var paths = spec.paths || {};
    Object.keys(paths).sort().forEach(function (path) {
      var item = paths[path] || {};
      ["get", "post", "put", "patch", "delete", "options", "head", "trace"].forEach(function (method) {
        if (!item[method]) {
          return;
        }
        var operation = item[method];
        var groups = operationGroups(operation, path);
        var tag = operation.tags && operation.tags.length ? operation.tags[0] : groups[groups.length - 1] || "默认分组";
        operations.push({
          key: method + " " + path,
          method: method,
          path: path,
          tag: tag,
          groups: groups,
          summary: operation.summary || "",
          description: operation.description || "",
          operationId: operation.operationId || "",
          parameters: (item.parameters || []).concat(operation.parameters || []),
          operation: operation
        });
      });
    });
    return operations;
  }

  function buildOperationTree(operations) {
    var root = { children: [], childMap: {} };
    operations.forEach(function (op) {
      var groups = op.groups && op.groups.length ? op.groups : ["默认分组"];
      var current = root;
      var path = [];
      groups.forEach(function (name) {
        path.push(name);
        var key = path.join("/");
        if (!current.childMap[name]) {
          current.childMap[name] = {
            name: name,
            key: key,
            count: 0,
            children: [],
            childMap: {},
            operations: []
          };
          current.children.push(current.childMap[name]);
        }
        current = current.childMap[name];
        current.count += 1;
      });
      current.operations.push(op);
    });
    sortOperationTree(root);
    return root;
  }

  function sortOperationTree(node) {
    node.children.sort(function (a, b) {
      return a.name.localeCompare(b.name);
    });
    node.children.forEach(sortOperationTree);
    if (node.operations) {
      node.operations.sort(function (a, b) {
        return (a.summary || a.path).localeCompare(b.summary || b.path);
      });
    }
  }

  function operationGroups(operation, path) {
    var groups = normalizeGroups(operation && operation["x-groups"]);
    if (groups.length) {
      return groups;
    }
    var tag = operation && operation.tags && operation.tags.length ? operation.tags[0] : "";
    groups = splitGroupPath(tag);
    if (groups.length) {
      return groups;
    }
    groups = pathGroups(path);
    return groups.length ? groups : ["默认分组"];
  }

  function normalizeGroups(value) {
    if (!Array.isArray(value)) {
      return [];
    }
    return value.map(function (item) {
      return String(item || "").trim();
    }).filter(Boolean);
  }

  function splitGroupPath(value) {
    return String(value || "").split(/[\/>]/).map(function (item) {
      return item.trim();
    }).filter(Boolean);
  }

  function pathGroups(path) {
    var parts = String(path || "").split("/").filter(Boolean);
    if (!parts.length) {
      return [];
    }
    return [parts[0].replace(/\{|\}/g, "") || "默认分组"];
  }

  function resolveRef(value) {
    if (!value) {
      return null;
    }
    if (!value.$ref) {
      return value;
    }
    return getByPointer(state.spec, value.$ref) || value;
  }

  function resolveSchema(value) {
    var schema = resolveRef(value);
    if (!schema) {
      return null;
    }
    if (schema.allOf && schema.allOf.length === 1 && schema.nullable) {
      var inner = resolveSchema(schema.allOf[0]);
      if (inner) {
        return Object.assign({}, inner, { nullable: true });
      }
    }
    return schema;
  }

  function getByPointer(root, ref) {
    if (!ref || ref.indexOf("#/") !== 0) {
      return null;
    }
    return ref.slice(2).split("/").reduce(function (current, part) {
      if (!current) {
        return null;
      }
      var key = part.replace(/~1/g, "/").replace(/~0/g, "~");
      return current[key];
    }, root);
  }

  function schemaLabel(schema) {
    if (!schema) {
      return "";
    }
    if (schema.$ref) {
      return schema.$ref.split("/").pop();
    }
    var label = schemaTypeLabel(schema);
    if (schema.nullable) {
      label += " nullable";
    }
    return label;
  }

  function schemaTypeLabel(schema) {
    if (!schema) {
      return "";
    }
    if (isFileSchema(schema)) {
      return "file";
    }
    if (schema.oneOf) {
      return "oneOf";
    }
    if (schema.anyOf) {
      return "anyOf";
    }
    if (schema.allOf) {
      return "allOf";
    }
    var labels = {
      object: "object",
      array: "array",
      string: "string",
      integer: "integer",
      number: "number",
      boolean: "boolean",
      null: "null"
    };
    var label = labels[schema.type] || schema.type || "object";
    if (schema.format) {
      return schemaFormatLabel(schema, label);
    }
    return label;
  }

  function schemaFormatLabel(schema, fallback) {
    if (!schema || !schema.format) {
      return fallback;
    }
    if ((schema.type === "integer" || schema.type === "number") && schema.format) {
      return schema.format;
    }
    if (schema.type === "string" && schema.format) {
      return schema.format;
    }
    return fallback + " " + schema.format;
  }

  function isFileSchema(schema) {
    return Boolean(schema && schema.type === "string" && (schema.format === "binary" || schema.format === "base64"));
  }

  function parameterLocationLabel(value) {
    var labels = {
      query: "查询",
      path: "路径",
      header: "请求头",
      cookie: "Cookie"
    };
    return labels[value] || value || "";
  }

  function firstServerURL(spec) {
    return spec.servers && spec.servers.length ? spec.servers[0].url || "" : "";
  }

  function demoSpec() {
    return {
      openapi: "3.0.3",
      info: {
        title: "Fox 示例接口",
        description: "用于验证 OpenAPI UI 展示效果的本地示例文档。",
        version: "1.0.0"
      },
      servers: [
        { url: "https://api.example.com", description: "线上环境" },
        { url: "http://localhost:8080", description: "本地环境" }
      ],
      paths: {
        "/users": {
          get: {
            tags: ["用户"],
            "x-groups": ["用户中心", "用户"],
            summary: "查询用户列表",
            operationId: "listUsers",
            parameters: [
              {
                name: "page",
                in: "query",
                description: "页码，从 1 开始。",
                required: false,
                schema: { type: "integer", minimum: 1, default: 1 }
              },
              {
                name: "pageSize",
                in: "query",
                description: "每页数量。",
                required: false,
                schema: { type: "integer", minimum: 1, maximum: 100, default: 20 }
              }
            ],
            responses: {
              "200": {
                description: "查询成功",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/UserListResponse" }
                  }
                }
              }
            }
          },
          post: {
            tags: ["用户"],
            "x-groups": ["用户中心", "用户"],
            summary: "创建用户",
            operationId: "createUser",
            requestBody: {
              required: true,
              content: {
                "application/json": {
                  schema: { $ref: "#/components/schemas/CreateUserRequest" }
                }
              }
            },
            responses: {
              "201": {
                description: "创建成功",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/UserResponse" }
                  }
                }
              },
              "400": {
                description: "请求参数错误",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/ErrorResponse" }
                  }
                }
              }
            }
          }
        },
        "/users/{id}": {
          get: {
            tags: ["用户"],
            "x-groups": ["用户中心", "用户"],
            summary: "获取用户详情",
            operationId: "getUser",
            parameters: [
              {
                name: "id",
                in: "path",
                description: "用户 ID。",
                required: true,
                schema: { type: "integer", format: "int64", minimum: 1 }
              }
            ],
            responses: {
              "200": {
                description: "查询成功",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/UserResponse" }
                  }
                }
              },
              "404": {
                description: "用户不存在",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/ErrorResponse" }
                  }
                }
              }
            }
          },
          patch: {
            tags: ["用户"],
            "x-groups": ["用户中心", "用户"],
            summary: "更新用户资料",
            operationId: "updateUser",
            parameters: [
              {
                name: "id",
                in: "path",
                description: "用户 ID。",
                required: true,
                schema: { type: "integer", format: "int64" }
              }
            ],
            requestBody: {
              required: true,
              content: {
                "application/json": {
                  schema: { $ref: "#/components/schemas/UpdateUserRequest" }
                }
              }
            },
            responses: {
              "200": {
                description: "更新成功",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/UserResponse" }
                  }
                }
              }
            }
          },
          delete: {
            tags: ["用户"],
            "x-groups": ["用户中心", "用户"],
            summary: "删除用户",
            operationId: "deleteUser",
            parameters: [
              {
                name: "id",
                in: "path",
                description: "用户 ID。",
                required: true,
                schema: { type: "integer", format: "int64" }
              }
            ],
            responses: {
              "204": { description: "删除成功" }
            }
          }
        },
        "/auth/login": {
          post: {
            tags: ["认证"],
            "x-groups": ["用户中心", "认证"],
            summary: "用户登录",
            operationId: "login",
            requestBody: {
              required: true,
              content: {
                "application/json": {
                  schema: { $ref: "#/components/schemas/LoginRequest" }
                }
              }
            },
            responses: {
              "200": {
                description: "登录成功",
                content: {
                  "application/json": {
                    schema: { $ref: "#/components/schemas/LoginResponse" }
                  }
                }
              }
            }
          }
        }
      },
      components: {
        schemas: {
          CreateUserRequest: {
            type: "object",
            required: ["name", "email"],
            properties: {
              name: { type: "string", description: "用户昵称。", minLength: 2, maxLength: 32 },
              email: { type: "string", format: "email", description: "邮箱地址。" },
              role: { type: "string", description: "用户角色。", enum: ["admin", "member"], default: "member" }
            }
          },
          UpdateUserRequest: {
            type: "object",
            properties: {
              name: { type: "string", description: "用户昵称。", minLength: 2, maxLength: 32 },
              status: { type: "string", description: "用户状态。", enum: ["active", "disabled"] }
            }
          },
          LoginRequest: {
            type: "object",
            required: ["email", "password"],
            properties: {
              email: { type: "string", format: "email", description: "邮箱地址。" },
              password: { type: "string", format: "password", description: "登录密码。", minLength: 8 }
            }
          },
          LoginResponse: {
            type: "object",
            required: ["accessToken", "expiresIn", "user"],
            properties: {
              accessToken: { type: "string", description: "访问令牌。" },
              expiresIn: { type: "integer", description: "过期时间，单位秒。" },
              user: { $ref: "#/components/schemas/User" }
            }
          },
          UserListResponse: {
            type: "object",
            required: ["items", "total"],
            properties: {
              items: {
                type: "array",
                description: "用户列表。",
                items: { $ref: "#/components/schemas/User" }
              },
              total: { type: "integer", description: "总数量。" }
            }
          },
          UserResponse: {
            type: "object",
            required: ["data"],
            properties: {
              data: { $ref: "#/components/schemas/User" }
            }
          },
          User: {
            type: "object",
            required: ["id", "name", "email", "status", "createdAt"],
            properties: {
              id: { type: "integer", format: "int64", description: "用户 ID。" },
              name: { type: "string", description: "用户昵称。" },
              email: { type: "string", format: "email", description: "邮箱地址。" },
              status: { type: "string", description: "用户状态。", enum: ["active", "disabled"] },
              createdAt: { type: "string", format: "date-time", description: "创建时间。" }
            }
          },
          ErrorResponse: {
            type: "object",
            required: ["code", "message"],
            properties: {
              code: { type: "string", description: "错误码。" },
              message: { type: "string", description: "错误信息。" },
              traceId: { type: "string", description: "链路追踪 ID。" }
            }
          }
        }
      }
    };
  }

  function buildCurl(op) {
    var contentType = requestBodyContentType(op);
    var requestBody = resolveRef(op.operation.requestBody);
    var media = requestBody && requestBody.content ? requestBody.content[contentType] || {} : {};
    var example = bodyExampleMap(op, contentType, media.example || (requestBody && requestBody.example) || {});
    var lines = ["curl -X " + op.method.toUpperCase() + " '" + buildRequestURL(op) + "'"];
    buildRequestHeaders(op, contentType.indexOf("multipart/form-data") >= 0 ? "" : contentType).forEach(function (header) {
      lines.push("  -H '" + header.name + ": " + (header.value || "") + "'");
    });
    if (contentType.indexOf("multipart/form-data") >= 0) {
      var schema = resolveSchema(media.schema);
      var properties = schema && schema.properties ? schema.properties : {};
      Object.keys(properties).forEach(function (name) {
        var child = resolveSchema(properties[name]) || properties[name];
        if (isFileSchema(child)) {
          lines.push("  -F '" + name + "=@/path/to/file'");
        } else if (example && example[name] !== undefined) {
          lines.push("  -F '" + name + "=" + String(example[name]).replace(/'/g, "'\\''") + "'");
        }
      });
      return lines.join(" \\\n");
    }
    if (contentType.indexOf("application/x-www-form-urlencoded") >= 0) {
      var encoded = formEncode(example);
      if (encoded) {
        lines.push("  --data-raw '" + encoded.replace(/'/g, "'\\''") + "'");
      }
    } else if (contentType.indexOf("json") >= 0) {
      lines.push("  --data-raw '" + JSON.stringify(example || {}, null, 2).replace(/'/g, "'\\''") + "'");
    }
    return lines.join(" \\\n");
  }

  function shortContentType(contentType) {
    if (contentType === "application/json") {
      return "JSON";
    }
    if (contentType === "application/xml" || contentType === "text/xml") {
      return "XML";
    }
    if (contentType === "application/x-www-form-urlencoded") {
      return "x-www-form-urlencoded";
    }
    return contentType;
  }

  function shortServerURL(value) {
    if (!value) {
      return "未配置";
    }
    try {
      var url = new URL(value, window.location.href);
      return url.host || value;
    } catch (err) {
      return value.replace(/^https?:\/\//, "").replace(/\/$/, "") || value;
    }
  }

  function methodClass(method) {
    return ["get", "post", "put", "patch", "delete"].indexOf(method) >= 0 ? method : "other";
  }

  function showToast(message) {
    var toast = document.getElementById("ui-toast");
    if (!toast) {
      toast = document.createElement("div");
      toast.id = "ui-toast";
      toast.className = "ui-toast";
      document.body.appendChild(toast);
    }
    toast.textContent = message;
    toast.classList.add("show");
    window.clearTimeout(showToast.timer);
    showToast.timer = window.setTimeout(function () {
      toast.classList.remove("show");
    }, 1800);
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).catch(function () {
        return fallbackCopyText(text);
      });
    }
    return fallbackCopyText(text);
  }

  function fallbackCopyText(text) {
    var input = document.createElement("textarea");
    input.value = text;
    input.setAttribute("readonly", "readonly");
    input.style.position = "fixed";
    input.style.top = "-999px";
    document.body.appendChild(input);
    input.select();
    document.execCommand("copy");
    document.body.removeChild(input);
    return Promise.resolve();
  }

  function updateThemeButton(theme) {
    if (!els.theme) {
      return;
    }
    els.theme.innerHTML = icon(theme === "dark" ? "sun" : "moon");
    els.theme.title = theme === "dark" ? "切换到亮色主题" : "切换到暗色主题";
    els.theme.setAttribute("aria-label", els.theme.title);
  }

  function renderError(err) {
    document.title = state.config.title || "OpenAPI 工作台";
    els.title.textContent = document.title;
    els.meta.textContent = "未加载接口文档";
    els.server.innerHTML = '<option>暂无环境</option>';
    els.server.disabled = true;
    els.summary.innerHTML = "";
    els.list.innerHTML = '<div class="sidebar-empty"><span class="sidebar-empty-icon" aria-hidden="true">' + icon("code") + '</span><strong>未加载接口</strong><small>请通过后端处理器提供文档，或在 index.html 同目录放置 openapi.json。</small></div>';
    els.tabs.innerHTML = "";
    els.content.innerHTML = renderWelcome(err);
  }

  function renderEmptyWorkbench() {
    var operationCount = state.operations.length;
    var groupCount = buildOperationTree(state.operations).children.length;
    return '<section class="select-empty-state">' +
      '<div class="empty-workbench">' +
      '<div class="empty-workbench-main">' +
      '<div class="empty-kicker"><span class="empty-kicker-dot"></span><span>OpenAPI 工作区</span></div>' +
      '<h2>选择接口后开始调试</h2>' +
      '<p>当前文档已加载 ' + operationCount + ' 个接口，分布在 ' + groupCount + ' 个分组中。接口详情会在这里打开，不会默认选中。</p>' +
      '<div class="empty-stats" aria-label="文档概览">' +
      '<div><strong>' + operationCount + '</strong><span>接口</span></div>' +
      '<div><strong>' + groupCount + '</strong><span>分组</span></div>' +
      '<div><strong>' + escapeHTML(shortServerURL(state.serverURL || "未配置")) + '</strong><span>环境</span></div>' +
      '</div>' +
      '<div class="empty-flow" aria-label="工作区内容">' +
      emptyFlowItem("list", "请求", "Params / Body / Header") +
      emptyFlowItem("terminal", "发送", "代理请求和 cURL") +
      emptyFlowItem("schema", "响应", "Body / Header / 实际请求") +
      '</div>' +
      '</div>' +
      '<aside class="empty-shortcuts" aria-label="接口快捷入口">' +
      '<div class="empty-shortcuts-head"><span>快速打开</span><small>' + operationCount + ' available</small></div>' +
      renderEmptyOperationShortcuts() +
      '</aside>' +
      '</div>' +
      '</section>';
  }

  function emptyFlowItem(iconName, title, text) {
    return '<div class="empty-flow-item"><span aria-hidden="true">' + icon(iconName) + '</span><strong>' + escapeHTML(title) + '</strong><small>' + escapeHTML(text) + '</small></div>';
  }

  function renderEmptyOperationShortcuts() {
    if (!state.operations.length) {
      return '<div class="empty-shortcuts-none">当前文档没有接口。</div>';
    }
    return state.operations.slice(0, 5).map(function (op) {
      return '<button class="empty-operation" type="button" data-empty-operation="' + escapeAttribute(op.key) + '">' +
        '<span class="method ' + methodClass(op.method) + '">' + op.method.toUpperCase() + '</span>' +
        '<span class="empty-operation-copy"><strong>' + escapeHTML(op.summary || op.operationId || op.path) + '</strong><code>' + escapeHTML(op.path) + '</code></span>' +
        '<span class="empty-operation-arrow" aria-hidden="true">' + icon("chevronDown") + '</span>' +
        '</button>';
    }).join("");
  }

  function bindEmptyWorkbenchActions() {
    Array.prototype.forEach.call(els.content.querySelectorAll("[data-empty-operation]"), function (button) {
      button.addEventListener("click", function () {
        selectOperation(button.dataset.emptyOperation || "");
      });
    });
  }

  function renderWelcome(err) {
    var message = err && err.message ? err.message : "未加载 OpenAPI 文档。";
    return '<section class="welcome-state">' +
      '<div class="welcome-hero">' +
      '<div class="hero-mark" aria-hidden="true">' + icon("code") + '</div>' +
      '<h2>从 OpenAPI 文档生成接口工作台</h2>' +
      '<p>集中浏览接口、查看请求结构、对比响应内容，并复制调试命令。</p>' +
      '<div class="welcome-actions">' +
      '<a class="button primary" href="' + escapeHTML(state.config.specUrl || "openapi.json") + '" target="_blank" rel="noreferrer"><span class="button-icon" aria-hidden="true">' + icon("file") + '</span><span>打开文档</span></a>' +
      '<button class="button secondary" type="button" disabled>加载失败：' + escapeHTML(message) + '</button>' +
      '</div>' +
      '</div>' +
      '<div class="quick-grid">' +
      quickTile("list", "接口树", "在左侧导航中搜索和切换接口。") +
      quickTile("schema", "结构查看", "查看请求体和嵌套响应结构。") +
      quickTile("terminal", "调试准备", "无需发送真实请求也能复制调试命令。") +
      quickTile("layout", "响应式布局", "宽屏和窄屏窗口都能舒适使用。") +
      '</div>' +
      '</section>';
  }

  function quickTile(iconName, title, text) {
    return '<article class="quick-tile"><div class="quick-mark" aria-hidden="true">' + icon(iconName) + '</div><h3>' + escapeHTML(title) + '</h3><p>' + escapeHTML(text) + '</p></article>';
  }

  function icon(name) {
    var paths = {
      chevronDown: '<path d="m6 9 6 6 6-6" />',
      code: '<path d="m16 18 6-6-6-6" /><path d="m8 6-6 6 6 6" /><path d="m14 4-4 16" />',
      copy: '<rect x="9" y="9" width="13" height="13" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />',
      file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" /><path d="M14 2v6h6M8 13h8M8 17h5" />',
      folder: '<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H10l2 2h6.5A2.5 2.5 0 0 1 21 9.5v7A2.5 2.5 0 0 1 18.5 19h-13A2.5 2.5 0 0 1 3 16.5z" />',
      grip: '<circle cx="9" cy="5" r="1" /><circle cx="15" cy="5" r="1" /><circle cx="9" cy="12" r="1" /><circle cx="15" cy="12" r="1" /><circle cx="9" cy="19" r="1" /><circle cx="15" cy="19" r="1" />',
      layout: '<rect x="3" y="3" width="18" height="18" rx="2" /><path d="M3 9h18M9 21V9" />',
      link: '<path d="M10 13a5 5 0 0 0 7.1 0l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1" /><path d="M14 11a5 5 0 0 0-7.1 0l-2 2A5 5 0 0 0 12 20.1l1.1-1.1" />',
      list: '<path d="M8 6h13M8 12h13M8 18h13" /><path d="M3 6h.01M3 12h.01M3 18h.01" />',
      minus: '<path d="M5 12h14" />',
      moon: '<path d="M21 12.8A8.5 8.5 0 1 1 11.2 3a6.5 6.5 0 0 0 9.8 9.8z" />',
      plus: '<path d="M12 5v14M5 12h14" />',
      rocket: '<path d="M14 4c-3.2.6-5.8 3-6.6 6.2L5 13l2.8-.1C11.3 12.2 13.8 9.7 14.5 6.7L18 3.2z" /><path d="M10.5 13.5 8 16" /><path d="M12.8 11.2 14.5 13" /><path d="M8.8 18.5 6.5 21l3.1-1 .4-1.5" />',
      schema: '<path d="M12 3v6M12 15v6M5 9h14M5 15h14" /><rect x="3" y="9" width="4" height="6" rx="1" /><rect x="10" y="9" width="4" height="6" rx="1" /><rect x="17" y="9" width="4" height="6" rx="1" />',
      sun: '<circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />',
      terminal: '<path d="m4 17 6-5-6-5" /><path d="M12 19h8" />',
      upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" /><path d="M17 8l-5-5-5 5" /><path d="M12 3v12" />',
      x: '<path d="M18 6 6 18M6 6l12 12" />'
    };
    return '<svg viewBox="0 0 24 24" role="img" focusable="false">' + (paths[name] || paths.code) + '</svg>';
  }

  function escapeAttribute(value) {
    return escapeHTML(value).replace(/`/g, "&#96;");
  }

  function escapeHTML(value) {
    return String(value == null ? "" : value).replace(/[&<>"']/g, function (ch) {
      return {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': "&quot;",
        "'": "&#39;"
      }[ch];
    });
  }
})();
