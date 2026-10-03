const timeline = document.querySelector("#timeline");
const timelineIndex = document.querySelector("#timeline-index");
const accountActions = document.querySelector("#account-actions");
const authDialog = document.querySelector("#auth-dialog");
const artifactDialog = document.querySelector("#artifact-dialog");
const artifactList = document.querySelector("#artifact-list");
const artifactEvent = document.querySelector("#artifact-event");
const artifactStatus = document.querySelector("#artifact-status");
const artifactUpload = document.querySelector("#artifact-upload");
const toast = document.querySelector("#toast");
const eraPanels = new Map();
let activeMedia = null;
let currentUser = null;
let eras = [];
let frames = new Map();
let donationsEnabled = false;
let toastTimer;

function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  const token = sessionStorage.getItem("little-years-session");
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (options.body && !(options.body instanceof FormData)) headers.set("Content-Type", "application/json");
  return fetch(`/api/v1${path}`, { ...options, headers }).then(async response => {
    const body = response.status === 204 ? null : await response.json().catch(() => null);
    if (!response.ok) throw new Error(body?.error || `Request failed (${response.status})`);
    return body;
  });
}

function node(tag, className, text) {
  const item = document.createElement(tag);
  if (className) item.className = className;
  if (text !== undefined) item.textContent = text;
  return item;
}

function showToast(message) {
  toast.textContent = message;
  toast.classList.add("visible");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toast.classList.remove("visible"), 2800);
}

function setAccount(user) {
  currentUser = user;
  accountActions.replaceChildren();
  if (!user) {
    const signIn = node("button", "text-button", "Family sign in ↗");
    signIn.type = "button";
    signIn.addEventListener("click", () => authDialog.showModal());
    accountActions.append(signIn);
    return;
  }
  const label = node("span", "text-button", user.name);
  accountActions.append(label);
  const artifacts = node("button", "text-button", "Artifacts");
  artifacts.type = "button";
  artifacts.addEventListener("click", openArtifactBrowser);
  accountActions.append(artifacts);
  artifactUpload.hidden = user.role !== "Admin";
  const signOut = node("button", "text-button", "Sign out");
  signOut.type = "button";
  signOut.addEventListener("click", async () => {
    try { await request("/auth/logout", { method: "POST", body: "{}" }); } catch {}
    sessionStorage.removeItem("little-years-session");
    setAccount(null);
    if (artifactDialog.open) artifactDialog.close();
    renderComments();
  });
  accountActions.append(signOut);
}

async function openArtifactBrowser() {
  artifactDialog.showModal();
  artifactList.replaceChildren();
  artifactStatus.textContent = "Loading events…";
  try {
    const events = await request("/events");
    artifactEvent.replaceChildren();
    events.forEach(event => artifactEvent.append(new Option(event.title, String(event.id))));
    artifactUpload.hidden = currentUser?.role !== "Admin" || events.length === 0;
    if (!events.length) {
      artifactStatus.textContent = "No events are available to browse.";
      return;
    }
    artifactEvent.onchange = loadArtifacts;
    await loadArtifacts();
  } catch (error) {
    artifactStatus.textContent = error.message;
  }
}

async function loadArtifacts() {
  artifactList.replaceChildren();
  artifactStatus.textContent = "Loading artifacts…";
  try {
    const artifacts = await request(`/artifacts?event_id=${encodeURIComponent(artifactEvent.value)}`);
    artifactStatus.textContent = artifacts.length ? `${artifacts.length} item${artifacts.length === 1 ? "" : "s"}` : "No artifacts for this event yet.";
    artifacts.forEach(artifact => {
      const row = node("article", "artifact-item");
      const details = node("div", "artifact-details");
      details.append(node("h3", "", artifact.title));
      details.append(node("p", "", artifact.description || artifact.file_name));
      details.append(node("small", "", `${artifact.file_name} · ${new Date(artifact.created_at + (artifact.created_at.endsWith("Z") ? "" : "Z")).toLocaleDateString()}`));
      const actions = node("div", "artifact-actions");
      const view = node("button", "text-button", "View / download");
      view.type = "button";
      view.addEventListener("click", () => viewArtifact(artifact));
      actions.append(view);
      if (currentUser?.role === "Admin") {
        const remove = node("button", "text-button", "Delete");
        remove.type = "button";
        remove.addEventListener("click", async () => {
          if (!confirm(`Delete “${artifact.title}”?`)) return;
          try {
            await request(`/artifacts/${artifact.id}`, { method: "DELETE" });
            await loadArtifacts();
          } catch (error) { showToast(error.message); }
        });
        actions.append(remove);
      }
      row.append(details, actions);
      artifactList.append(row);
    });
  } catch (error) {
    artifactStatus.textContent = error.message;
  }
}

async function viewArtifact(artifact) {
  try {
    const response = await fetch(`/api/v1/artifacts/${artifact.id}/file`, {
      headers: { Authorization: `Bearer ${sessionStorage.getItem("little-years-session")}` },
    });
    if (!response.ok) {
      const error = await response.json().catch(() => null);
      throw new Error(error?.error || `Download failed (${response.status})`);
    }
    const objectURL = URL.createObjectURL(await response.blob());
    const opened = window.open(objectURL, "_blank", "noopener,noreferrer");
    if (!opened) {
      const link = node("a", "artifact-download", "Open artifact");
      link.href = objectURL;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      artifactStatus.replaceChildren(document.createTextNode("Pop-up blocked. "), link);
    }
    setTimeout(() => URL.revokeObjectURL(objectURL), 60_000);
  } catch (error) { showToast(error.message); }
}

artifactUpload.addEventListener("submit", async event => {
  event.preventDefault();
  const form = event.currentTarget;
  const data = new FormData(form);
  data.set("event_id", artifactEvent.value);
  try {
    await request("/artifacts", { method: "POST", body: data });
    form.reset();
    await loadArtifacts();
    showToast("Artifact added");
  } catch (error) { showToast(error.message); }
});

document.querySelector(".artifact-close").addEventListener("click", () => artifactDialog.close());
artifactDialog.addEventListener("click", event => { if (event.target === artifactDialog) artifactDialog.close(); });

function applyTheme(theme = {}) {
  const root = document.documentElement;
  for (const [key, value] of [["--accent", theme.accent], ["--paper", theme.bg]]) {
    if (typeof value === "string" && /^#[\da-f]{6}$/i.test(value)) root.style.setProperty(key, value);
  }
  if (["Fraunces", "DM Sans", "Georgia", "serif", "sans-serif"].includes(theme.font)) {
    root.style.setProperty("--display", theme.font === "DM Sans" ? '"DM Sans", sans-serif' : theme.font === "Fraunces" ? '"Fraunces", Georgia, serif' : theme.font);
  }
  document.querySelector('meta[name="theme-color"]').content = getComputedStyle(root).getPropertyValue("--paper").trim();
}

function feature(region, era, index) {
  const frame = frames.get(region);
  if (!frame || !frame.visible || frame.feature === "empty") return null;
  const config = frame.config || {};
  if (frame.feature === "timeline") return timelineFeature(index);
  if (frame.feature === "era_title") return titleFeature(era, index, config);
  if (frame.feature === "album_grid") return albumFeature(era, config);
  if (frame.feature === "comments") return commentsFeature(config);
  if (frame.feature === "donate_qr") return donationsEnabled ? donateFeature(config) : null;
  if (frame.feature === "media_gallery") return albumFeature(era, config);
  return null;
}

function timelineFeature(index) {
  const wrap = node("div", "era-index", `LITTLE YEARS · ${String(index + 1).padStart(2, "0")}`);
  const links = node("div", "timeline-links");
  eras.forEach((era, eraIndex) => {
    const button = node("button", "", era.title);
    button.type = "button";
    button.dataset.eraIndex = String(eraIndex);
    button.setAttribute("aria-current", String(eraIndex === index));
    button.addEventListener("click", () => eraPanels.get(era.id)?.scrollIntoView({ behavior: "smooth", inline: "start", block: "nearest" }));
    links.append(button);
  });
  wrap.append(links);
  return wrap;
}

function titleFeature(era, index, config = {}) {
  const content = node("div");
  content.append(node("p", "eyebrow", config.label || `ERA ${String(index + 1).padStart(2, "0")}`));
  content.append(node("h1", "era-heading", era.title));
  content.append(node("div", "era-rule"));
  content.append(node("p", "era-meta", `${era.slug.replaceAll("-", " ")}  /  family archive`));
  return content;
}

function albumFeature(era, config = {}) {
  const stack = node("div", "album-stack");
  const columns = Number(config.columns);
  if (Number.isInteger(columns) && columns >= 1 && columns <= 4) stack.style.setProperty("--album-columns", String(columns));
  if (config.showCaptions === false) stack.dataset.showCaptions = "false";
  const observer = new IntersectionObserver(entries => {
    for (const entry of entries) {
      if (!entry.isIntersecting || entry.target.dataset.loaded) continue;
      entry.target.dataset.loaded = "true";
      observer.unobserve(entry.target);
      loadAlbums(era.slug, entry.target);
    }
  }, { root: timeline, rootMargin: "160px" });
  observer.observe(stack);
  return stack;
}

async function loadAlbums(slug, stack) {
  try {
    const albums = await request(`/eras/${encodeURIComponent(slug)}/albums`);
    if (!albums.length) {
      stack.append(node("p", "empty-era", "The pages are ready for their first photographs."));
      return;
    }
    for (const album of albums) {
      const section = node("section", "album");
      const heading = node("h2", "album-title", album.title);
      heading.append(node("small", "", String(album.sort_order).padStart(2, "0")));
      section.append(heading);
      const grid = node("div", "media-grid");
      if (stack.dataset.showCaptions === "false") grid.classList.add("captions-hidden");
      section.append(grid);
      stack.append(section);
      loadMedia(album.id, grid);
    }
  } catch (error) {
    stack.append(node("p", "empty-era", error.message));
  }
}

async function loadMedia(albumID, grid) {
  try {
    const media = await request(`/albums/${albumID}/media`);
    if (!media.length) {
      grid.append(node("p", "album-empty", "A quiet page, for now."));
      return;
    }
    media.forEach(item => {
      const figure = node("figure", "media-tile");
      const visual = item.kind === "short" ? node("video") : node("img");
      visual.src = item.kind === "short" ? item.url : item.thumb_url;
      if (item.kind === "short") { visual.poster = item.thumb_url; visual.controls = true; visual.preload = "none"; }
      else { visual.alt = item.caption || "A family memory"; visual.loading = "lazy"; }
      figure.append(visual);
      figure.append(node("figcaption", "", item.caption || "Open comments"));
      figure.addEventListener("click", event => {
        if (event.target instanceof HTMLVideoElement) return;
        activeMedia = item;
        renderComments(figure.closest(".era-panel")?.querySelector("[data-comments]"));
      });
      grid.append(figure);
    });
  } catch (error) { grid.append(node("p", "album-empty", error.message)); }
}

function commentsFeature(config = {}) {
  const section = node("section", "comments-panel");
  section.dataset.comments = "true";
  section.dataset.heading = typeof config.heading === "string" ? config.heading : "Notes from home";
  renderComments(section);
  return section;
}

async function renderComments(target = document.querySelector("[data-comments]")) {
  if (!target) return;
  target.replaceChildren();
  target.append(node("h2", "comments-title", target.dataset.heading || "Notes from home"));
  if (!activeMedia) {
    target.append(node("p", "comments-intro", "Choose a photograph to read or leave a note."));
    return;
  }
  target.append(node("p", "comments-intro", activeMedia.caption || "A small moment, kept close."));
  const list = node("div", "comment-list");
  target.append(list);
  if (!currentUser || currentUser.role === "Visitor") {
    list.append(node("p", "comment-empty", "Family notes are shared with registered members."));
    const button = node("button", "text-button", "Sign in to join the conversation ↗");
    button.type = "button";
    button.addEventListener("click", () => authDialog.showModal());
    target.append(button);
    return;
  }
  try {
    const comments = await request(`/media/${activeMedia.id}/comments`);
    if (!comments.length) list.append(node("p", "comment-empty", "Be the first to leave a note."));
    comments.forEach(comment => {
      const item = node("article", "comment");
      item.append(node("p", "comment-name", comment.name));
      item.append(node("p", "comment-copy", comment.body));
      list.append(item);
    });
  } catch (error) { list.append(node("p", "comment-empty", error.message)); }
  const form = node("form", "comment-form");
  const input = node("textarea");
  input.maxLength = 4000;
  input.required = true;
  input.placeholder = "Leave a little note…";
  const submit = node("button", "button button-dark", "Leave note ↗");
  submit.type = "submit";
  form.append(input, submit);
  form.addEventListener("submit", async event => {
    event.preventDefault();
    try {
      await request(`/media/${activeMedia.id}/comments`, { method: "POST", body: JSON.stringify({ body: input.value }) });
      renderComments(target);
    } catch (error) { showToast(error.message); }
  });
  target.append(form);
}

function donateFeature(config = {}) {
  const wrap = node("div", "donate-slot");
  const qr = node("img");
  qr.src = "/api/v1/donate/qr";
  qr.alt = "Scan to support the family archive";
  qr.addEventListener("error", () => qr.remove(), { once: true });
  const link = node("a", "", config.label || "Support this archive ↗");
  link.href = "/api/v1/donate/link";
  link.target = "_blank";
  link.rel = "noopener noreferrer";
  wrap.append(qr, link);
  return wrap;
}

function renderEra(era, index) {
  const panel = node("article", "era-panel");
  panel.dataset.eraId = String(era.id);
  panel.dataset.theme = JSON.stringify(era.theme || {});
  const slots = new Map();
  for (const region of ["top", "left", "center", "right", "bottom"]) {
    const slot = node("section", `frame frame-${region}`);
    slot.dataset.region = region;
    const content = feature(region, era, index);
    if (content) slot.append(content);
    slots.set(region, slot);
  }
  slots.get("bottom").append(node("span", "timeline-count", `${String(index + 1).padStart(2, "0")} / ${String(eras.length).padStart(2, "0")}`));
  panel.append(...slots.values());
  return panel;
}

function activatePanel(panel) {
  const theme = JSON.parse(panel.dataset.theme || "{}");
  applyTheme(theme);
  timelineIndex.querySelectorAll("button").forEach((button, index) => button.setAttribute("aria-current", String(index === eras.findIndex(era => String(era.id) === panel.dataset.eraId))));
  const activeIndex = eras.findIndex(era => String(era.id) === panel.dataset.eraId);
  document.querySelectorAll(".timeline-links button").forEach(button => button.setAttribute("aria-current", String(Number(button.dataset.eraIndex) === activeIndex)));
}

function observePanels() {
  const observer = new IntersectionObserver(entries => {
    for (const entry of entries) if (entry.isIntersecting) activatePanel(entry.target);
  }, { root: timeline, threshold: .65 });
  timeline.querySelectorAll(".era-panel").forEach(panel => observer.observe(panel));
}

async function start() {
  const token = new URLSearchParams(location.search).get("token");
  if (token) {
    try {
      const session = await request("/auth/verify", { method: "POST", body: JSON.stringify({ token }) });
      sessionStorage.setItem("little-years-session", session.token);
      history.replaceState({}, "", location.pathname);
      setAccount(session.user);
      showToast("You’re signed in. Come on in.");
    } catch (error) { showToast(error.message); }
  }
  const savedToken = sessionStorage.getItem("little-years-session");
  if (savedToken) {
    try { setAccount(await request("/auth/me")); }
    catch { sessionStorage.removeItem("little-years-session"); setAccount(null); }
  } else setAccount(null);

  const [eraList, layoutFrames, donationStatus] = await Promise.all([
    request("/eras"),
    request("/layout").then(values => new Map(values.map(frame => [frame.region, frame]))),
    request("/donate/status"),
  ]);
  eras = eraList;
  frames = layoutFrames;
  donationsEnabled = donationStatus.enabled;
  const state = document.querySelector("#loading-state");
  state.remove();
  if (!eras.length) {
    timeline.append(node("div", "loading-state", "Your first chapter is waiting for its first page."));
    return;
  }
  eras.forEach((era, index) => {
    const panel = renderEra(era, index);
    eraPanels.set(era.id, panel);
    timeline.append(panel);
    const dot = node("button");
    dot.type = "button";
    dot.setAttribute("aria-label", `Go to ${era.title}`);
    dot.addEventListener("click", () => panel.scrollIntoView({ behavior: "smooth", inline: "start", block: "nearest" }));
    timelineIndex.append(dot);
  });
  observePanels();
  activatePanel(timeline.querySelector(".era-panel"));
  timeline.addEventListener("scroll", () => {
    const max = timeline.scrollWidth - timeline.clientWidth;
    document.querySelector("#progress-fill").style.width = `${max > 0 ? timeline.scrollLeft / max * 100 : 100}%`;
  }, { passive: true });
}

document.querySelector("#magic-form").addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const message = document.querySelector("#magic-message");
  message.textContent = "Sending your one-time link…";
  try {
    await request("/auth/magic-link", { method: "POST", body: JSON.stringify({ name: form.get("name"), email: form.get("email") }) });
    message.textContent = "If this address can join the archive, a sign-in link is on its way.";
  } catch (error) { message.textContent = error.message; }
});

document.querySelector(".dialog-close").addEventListener("click", () => authDialog.close());
authDialog.addEventListener("click", event => { if (event.target === authDialog) authDialog.close(); });
start().catch(error => {
  document.querySelector("#loading-state").replaceChildren(node("p", "", `The archive could not load: ${error.message}`));
});