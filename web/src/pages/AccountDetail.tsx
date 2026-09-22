import { useEffect, useLayoutEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  api,
  type Account,
  type Alias,
  type AliasMetadata,
  type BatchCreateResult,
  type FullMailMessage,
  type InboxData,
  type MailMessage,
  type MailReadPreference,
  type OrganizerGroup,
  type ShareLink,
} from "../api/client";
import GroupFilter from "../components/GroupFilter";
import Icon from "../components/Icon";
import MailHTMLFrame from "../components/MailHTMLFrame";
import PageLayout from "../components/PageLayout";
import Pagination from "../components/Pagination";
import PasswordInput from "../components/PasswordInput";
import SelectMenu from "../components/SelectMenu";
import { Dialog, SidePanel } from "../components/Overlay";

type Tab = "aliases" | "disabled" | "inbox" | "shares";
type ShareFilter = "all" | "active" | "expired";
type RefreshInterval = 0 | 5000 | 15000 | 30000;
type MailRangeDays = 0 | 7;
const PAGE_SIZE = 20;
const INBOX_COUNT_CACHE_MS = 60_000;
const MAX_SHARE_EXPIRY_MINUTES = 5_256_000;
const inboxCountCache = new Map<string, { count: number; expiresAt: number }>();
const mailMethodLabel = (method?: InboxData["method"]) => {
  if (method === "forward_imap") return "转发邮箱 IMAP（列表 + 正文）";
  if (method === "imap") return "iCloud IMAP（列表 + 正文）";
  if (method === "web_api") return "iCloud Web API（仅列表摘要）";
  return "未确定";
};
const parseDateValue = (date?: string) => {
  if (!date?.trim()) return null;
  const value = date.trim();
  const numeric = Number(value);
  const parsed = /^\d+(?:\.\d+)?$/.test(value)
    ? new Date(numeric >= 1_000_000_000_000 ? numeric : numeric * 1000)
    : new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
};
const dateText = (date?: string) => {
  const parsed = parseDateValue(date);
  return parsed
    ? parsed.toLocaleString("zh-CN", {
        dateStyle: "medium",
        timeStyle: "short",
      })
    : "—";
};
const compactDateText = (date?: string) => {
  const parsed = parseDateValue(date);
  if (!parsed) return "—";
  const parts = new Intl.DateTimeFormat("zh-CN", {
    year: "2-digit",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).formatToParts(parsed);
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((item) => item.type === type)?.value || "";
  return `${part("year")}-${part("month")}-${part("day")} ${part("hour")}:${part("minute")}`;
};
const errorText = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const newestFirst = <T extends { date?: string; id: string }>(items: T[]) =>
  [...items].sort((left, right) => {
    const leftTime = Date.parse(left.date || "");
    const rightTime = Date.parse(right.date || "");
    if (Number.isFinite(leftTime) && Number.isFinite(rightTime) && leftTime !== rightTime) {
      return rightTime - leftTime;
    }
    return right.id.localeCompare(left.id);
  });
const normalizeInbox = (value: InboxData): InboxData => ({
  ...value,
  messages: newestFirst(value.messages || []),
});
const copyCode = async (code: string, setNotice: (value: string) => void) => {
  try {
    await navigator.clipboard.writeText(code);
    setNotice("验证码已复制。");
  } catch {
    setNotice("验证码复制失败，请手动选择复制。");
  }
};
const urlPattern = /(https?:\/\/[^\s<]+)/g;
function MailBody({ text, html = false }: { text: string; html?: boolean }) {
  if (html && text.trim()) {
    return <MailHTMLFrame html={text} />;
  }
  const paragraphs = text.replace(/\r\n?/g, "\n").split(/\n{2,}/);
  return (
    <div className="mail-body-content">
      {paragraphs.map((paragraph, paragraphIndex) => (
        <p
          className={paragraph.split("\n").every((line) => line.trim().startsWith(">")) ? "mail-quote" : ""}
          key={paragraphIndex}
        >
          {paragraph.split("\n").map((line, lineIndex) => (
            <span key={lineIndex}>
              {line.split(urlPattern).map((part, index) => {
                if (!/^https?:\/\//i.test(part)) return <span key={index}>{part}</span>;
                const match = part.match(/^(.*?)([),.;!?，。；！）]*)$/);
                const url = match?.[1] || part;
                const suffix = match?.[2] || "";
                return (
                  <span key={index}>
                    <a href={url} target="_blank" rel="noreferrer">{url}</a>
                    {suffix}
                  </span>
                );
              })}
              {lineIndex < paragraph.split("\n").length - 1 && <br />}
            </span>
          ))}
        </p>
      ))}
    </div>
  );
}
function mailKey(item: Pick<MailMessage, "id" | "folder">) {
  return `${item.folder || "INBOX"}:${item.id}`;
}
const countCacheKey = (accountId: string, email: string) =>
  `${accountId}\n${email}`;
const cachedInboxCount = (accountId: string, email: string) => {
  const cached = inboxCountCache.get(countCacheKey(accountId, email));
  return cached && cached.expiresAt > Date.now() ? cached.count : null;
};
const rememberInboxCount = (accountId: string, email: string, count: number) =>
  inboxCountCache.set(countCacheKey(accountId, email), {
    count,
    expiresAt: Date.now() + INBOX_COUNT_CACHE_MS,
  });
const parseDurationMinutes = (raw: string): number | null => {
  const value = raw.trim().toLowerCase();
  if (!value) return null;
  if (value === "0" || value === "永久" || value === "permanent") return 0;
  const compact = value.replace(/\s+/g, "");
  const pattern =
    /(\d+(?:\.\d+)?)(minutes?|mins?|min|m|hours?|hrs?|hr|h|days?|d|weeks?|w)/g;
  const multipliers: Record<string, number> = {
    m: 1,
    min: 1,
    mins: 1,
    minute: 1,
    minutes: 1,
    h: 60,
    hr: 60,
    hrs: 60,
    hour: 60,
    hours: 60,
    d: 1_440,
    day: 1_440,
    days: 1_440,
    w: 10_080,
    week: 10_080,
    weeks: 10_080,
  };
  let cursor = 0;
  let total = 0;
  let matches = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(compact)) !== null) {
    if (match.index !== cursor) return null;
    total += Number(match[1]) * multipliers[match[2]];
    cursor = pattern.lastIndex;
    matches += 1;
  }
  if (
    !matches ||
    cursor !== compact.length ||
    !Number.isFinite(total) ||
    !Number.isInteger(total) ||
    total < 0 ||
    total > MAX_SHARE_EXPIRY_MINUTES
  )
    return null;
  return total;
};
const isShareExpired = (item: ShareLink) =>
  Boolean(item.expires_at && Date.parse(item.expires_at) <= Date.now());
const isShareDurationPreset = (value: string) =>
  ["0", "120", "10080", "20160"].includes(value);

function ShareRows({
  groups,
  selectedShares,
  onToggle,
  onCopy,
  onEdit,
  onDelete,
  onDisable,
}: {
  groups: Array<[string, ShareLink[]]>;
  selectedShares: string[];
  onToggle: (token: string) => void;
  onCopy: (item: ShareLink) => void;
  onEdit: (item: ShareLink) => void;
  onDelete: (item: ShareLink) => void;
  onDisable: (alias: string) => void;
}) {
  return (
    <>
      {groups.map(([alias, items]) => (
        <section className="share-group" key={alias}>
          <div className="share-group-heading">
            <strong className="mono">{alias}</strong>
            <span>{items.length} 个链接</span>
            <button
              className="text-button danger-text"
              onClick={() => onDisable(alias)}
            >
              停用邮箱
            </button>
          </div>
          <div className="share-group-items">
            {items.map((item) => {
              const expired = isShareExpired(item);
              return (
                <div
                  className={`share-row share-row-child ${expired ? "share-row-expired" : ""}`}
                  key={item.token}
                >
                  <label className="checkbox-hit share-select">
                    <input
                      type="checkbox"
                      aria-label={`选择分享链接 ${item.alias}`}
                      checked={selectedShares.includes(item.token)}
                      onChange={() => onToggle(item.token)}
                    />
                  </label>
                  <div className="share-row-main">
                    <div className="share-row-heading">
                      <strong>{item.label || "未命名链接"}</strong>
                      <span
                        className={`status status-${expired ? "error" : "ready"} share-status`}
                      >
                        {expired ? "已过期" : "有效"}
                      </span>
                    </div>
                    <span>
                      {expired
                        ? `已过期于 ${dateText(item.expires_at)}`
                        : item.expires_at
                          ? `有效至 ${dateText(item.expires_at)}`
                          : "永久有效"}{" "}
                      · 创建于 {dateText(item.created_at)}
                    </span>
                  </div>
                  <div>
                    <button
                      className="button small secondary"
                      onClick={() => onEdit(item)}
                    >
                      <Icon name="settings" size={15} />
                      编辑备注
                    </button>
                    <button
                      className="button small secondary"
                      onClick={() => onCopy(item)}
                    >
                      <Icon name="copy" size={15} />
                      复制
                    </button>
                    <button
                      className="text-button danger-text"
                      onClick={() => onDelete(item)}
                    >
                      删除
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      ))}
    </>
  );
}

function InlineMailRow({
  item,
  selected,
  checked,
  loading,
  error,
  method,
  onOpen,
  onClose,
  onRetry,
  onCopyCode,
  onDelete,
  onToggle,
}: {
  item: MailMessage;
  selected: FullMailMessage | null;
  checked: boolean;
  loading: boolean;
  error: string;
  method: MailReadPreference;
  onOpen: () => void;
  onClose: () => void;
  onRetry: () => void;
  onCopyCode: (code: string) => void;
  onDelete: () => void;
  onToggle: () => void;
}) {
  const expanded = selected?.id === item.id && selected.folder === item.folder;
  const toggleExpanded = () => {
    if (expanded) onClose();
    else onOpen();
  };
  return (
    <div className={`mail-item ${expanded ? "expanded" : ""}`}>
      <div
        className={`mail-row mail-row-button ${expanded ? "selected" : ""} ${method !== "web_api" ? "has-selection" : ""}`}
        onClick={toggleExpanded}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            toggleExpanded();
          }
        }}
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
      >
        {method !== "web_api" && (
          <input
            className="mail-select"
            type="checkbox"
            checked={checked}
            aria-label={`选择邮件：${item.subject || "无主题"}`}
            onChange={(event) => {
              event.stopPropagation();
              onToggle();
            }}
            onClick={(event) => event.stopPropagation()}
          />
        )}
        <div className="mail-avatar">
          {(item.from || "?").slice(0, 1).toUpperCase()}
        </div>
        <div>
          <strong>{item.subject || "（无主题）"}</strong>
          <span className="mail-route">发件人：{item.from || "未知"}{item.to ? ` · 收件人：${item.to}` : ""}</span>
          {item.preview && <small>{item.preview}</small>}
          {item.code && (
            <button
              className="mail-code-button"
              type="button"
              aria-label={`复制验证码 ${item.code}`}
              title="点击复制验证码"
              onClick={(event) => {
                event.stopPropagation();
                onCopyCode(item.code || "");
              }}
            >
              <span>验证码：{item.code}</span>
              <Icon name="copy" size={16} />
            </button>
          )}
        </div>
        <time>{dateText(item.date)}</time>
      </div>
      {expanded && (
        <div className="inline-mail-detail" aria-live="polite">
          <div className="inline-mail-detail-head">
            <div>
              <span className="eyebrow">邮件正文</span>
              <h3>{selected.subject || "（无主题）"}</h3>
              <p>
                发件人：{selected.from} · 收件人：{selected.to || "未知"} · {dateText(selected.date)}
              </p>
            </div>
            <div className="inline-actions">
              <button className="button small secondary" onClick={onClose}>
                收起
              </button>
              {selected.code && (
                <button
                  className="button small secondary"
                  onClick={() => onCopyCode(selected.code || "")}
                >
                  <Icon name="copy" size={15} />
                  复制验证码
                </button>
              )}
              <button className="button small danger-outline" onClick={onDelete}>
                <Icon name="trash" size={15} />
                删除邮件
              </button>
            </div>
          </div>
          {loading ? (
            <div className="inbox-reader-state" role="status">
              <span className="message-loading-bar" />
              <strong>正在读取正文</strong>
            </div>
          ) : error ? (
            <div className="inbox-reader-state error-state" role="alert">
              <strong>正文读取失败</strong>
              <small>{error}</small>
              <button className="button secondary" onClick={onRetry}>
                重新读取
              </button>
            </div>
          ) : method === "web_api" ? (
            <div className="inline-mail-body">
              <MailBody text={selected.preview || "这封邮件没有可显示的摘要。"} />
            </div>
          ) : (
            <div className="inline-mail-body">
              <MailBody text={selected.body || "这封邮件没有可显示的正文。"} html={selected.content_type === "text/html"} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}
export default function AccountDetailPage({
  onLogout,
}: {
  onLogout: () => void;
}) {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedAlias = searchParams.get("alias") || "";
  const requestedTabValue = searchParams.get("tab");
  const requestedTab: Tab =
    requestedTabValue === "disabled" ||
    requestedTabValue === "inbox" ||
    requestedTabValue === "shares"
      ? requestedTabValue
      : "aliases";
  const requestedAliasPage = Math.max(
    1,
    Number.parseInt(searchParams.get("page") || "1", 10) || 1,
  );
  const [account, setAccount] = useState<Account | null>(null);
  const [aliases, setAliases] = useState<Alias[]>([]);
  const [shares, setShares] = useState<ShareLink[]>([]);
  const [inbox, setInbox] = useState<InboxData | null>(null);
  const [inboxCount, setInboxCount] = useState<number | null>(null);
  const [inboxPage, setInboxPage] = useState(1);
  const [inboxQuery, setInboxQuery] = useState("");
  const [groups, setGroups] = useState<OrganizerGroup[]>([]);
  const [metadata, setMetadata] = useState<Record<string, AliasMetadata>>({});
  const [aliasesLoading, setAliasesLoading] = useState(true);
  const [newAliasGroupId, setNewAliasGroupId] = useState("");
  const [tab, setTab] = useState<Tab>(requestedTab);
  const [query, setQuery] = useState("");
  const [alias, setAlias] = useState(requestedAlias);
  const [mailMethod, setMailMethod] = useState<MailReadPreference>("web_api");
  const [mailMethodNotice, setMailMethodNotice] = useState("");
  const [mailRangeDays, setMailRangeDays] = useState<MailRangeDays>(7);
  const [autoRefreshMs, setAutoRefreshMs] = useState<RefreshInterval>(() => {
    const saved = Number(localStorage.getItem("mail-auto-refresh-ms"));
    return [0, 5000, 15000, 30000].includes(saved) ? (saved as RefreshInterval) : 0;
  });
  const [label, setLabel] = useState("");
  const [groupFilter, setGroupFilter] = useState("all");
  const [aliasPage, setAliasPage] = useState(requestedAliasPage);
  const [selectedActive, setSelectedActive] = useState<string[]>([]);
  const [selectedDisabled, setSelectedDisabled] = useState<string[]>([]);
  const [activeDeleteConfirm, setActiveDeleteConfirm] = useState(false);
  const [disabledDeleteConfirm, setDisabledDeleteConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [createdAlias, setCreatedAlias] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [batchOpen, setBatchOpen] = useState(false);
  const [batchCount, setBatchCount] = useState(5);
  const [batchPrefix, setBatchPrefix] = useState("");
  const [batchNamingRule, setBatchNamingRule] = useState<"sequence" | "same">("sequence");
  const [batchSeparator, setBatchSeparator] = useState("-");
  const [batchStartNumber, setBatchStartNumber] = useState(1);
  const [batchPadding, setBatchPadding] = useState(3);
  const [batchGroupId, setBatchGroupId] = useState("");
  const [batchNote, setBatchNote] = useState("");
  const [batchResult, setBatchResult] = useState<BatchCreateResult | null>(null);
  const [mailConfigOpen, setMailConfigOpen] = useState(false);
  const [mailConfigTab, setMailConfigTab] = useState<"imap" | "forward_imap">("imap");
  const [organizerOpen, setOrganizerOpen] = useState(false);
  const [editorAlias, setEditorAlias] = useState<Alias | null>(null);
  const [editorLabel, setEditorLabel] = useState("");
  const [editorGroupId, setEditorGroupId] = useState("");
  const [editorNote, setEditorNote] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<Alias | null>(null);
  const [groupDeleteTarget, setGroupDeleteTarget] =
    useState<OrganizerGroup | null>(null);
  const [groupName, setGroupName] = useState("");
  const [renameGroupId, setRenameGroupId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [groupOrderDirty, setGroupOrderDirty] = useState(false);
  const [draggedGroupId, setDraggedGroupId] = useState<string | null>(null);
  const [dragOverGroupId, setDragOverGroupId] = useState<string | null>(null);
  const groupListRef = useRef<HTMLDivElement>(null);
  const groupRowRefs = useRef(new Map<string, HTMLDivElement>());
  const pendingGroupRects = useRef(new Map<string, DOMRect>());
  const groupMoveAnimations = useRef(new Map<string, Animation>());
  const groupDrag = useRef<{
    pointerId: number;
    sourceId: string;
    startY: number;
    pointerY: number;
    grabOffsetY: number;
    translateY: number;
  } | null>(null);
  const [message, setMessage] = useState<FullMailMessage | null>(null);
  const [messageLoading, setMessageLoading] = useState(false);
  const [messageError, setMessageError] = useState("");
  const [messageDeleting, setMessageDeleting] = useState(false);
  const [messageDeleteConfirm, setMessageDeleteConfirm] = useState(false);
  const [selectedMessages, setSelectedMessages] = useState<string[]>([]);
  const [messageBatchDeleteConfirm, setMessageBatchDeleteConfirm] =
    useState(false);
  const messageRequest = useRef(0);
  const inboxRequest = useRef(0);
  const autoRefreshRunning = useRef(false);
  const deletingMessages = useRef(new Set<string>());
  const [shareOpen, setShareOpen] = useState(false);
  const [shareAlias, setShareAlias] = useState("");
  const [shareLabel, setShareLabel] = useState("");
  const [shareDuration, setShareDuration] = useState("0");
  const [shareResult, setShareResult] = useState<string | null>(null);
  const [shareCreating, setShareCreating] = useState(false);
  const [shareQuery, setShareQuery] = useState("");
  const [shareFilter, setShareFilter] = useState<ShareFilter>("all");
  const [selectedShares, setSelectedShares] = useState<string[]>([]);
  const [shareDeleteConfirm, setShareDeleteConfirm] = useState(false);
  const [shareEditTarget, setShareEditTarget] = useState<ShareLink | null>(null);
  const [shareEditLabel, setShareEditLabel] = useState("");

  const visibleInboxMessages = useMemo(() => {
    const normalized = inboxQuery.trim().toLocaleLowerCase();
    if (!normalized) return inbox?.messages || [];
    return (inbox?.messages || []).filter((item) =>
      [item.subject, item.from, item.to, item.preview, item.code, item.date]
        .filter(Boolean)
        .some((value) => String(value).toLocaleLowerCase().includes(normalized)),
    );
  }, [inbox?.messages, inboxQuery]);

  const load = async () => {
    setBusy(true);
    setAliasesLoading(true);
    try {
      const [accounts, aliasResult, shareResultData, organizer] =
        await Promise.all([
          api.listAccounts(),
          api.listAliases(id),
          api.listShares(id),
          api.getOrganizer(id),
        ]);
      const nextAccount = accounts.find((item) => item.id === id) || null;
      setAccount(nextAccount);
      setMailMethod(nextAccount?.mail_read_method || "web_api");
      setAliases(aliasResult.aliases || []);
      setShares(shareResultData || []);
      setGroups(organizer.groups || []);
      setGroupOrderDirty(false);
      setMetadata(organizer.metadata || {});
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
      setAliasesLoading(false);
    }
  };
  useEffect(() => {
    load();
  }, [id]);
  useEffect(() => {
    if (requestedTab !== "inbox" || !account) return;
    const request = ++inboxRequest.current;
    setTab("inbox");
    setAlias(requestedAlias);
    setInboxCount(cachedInboxCount(id, requestedAlias));
    setBusy(true);
    api
      .inbox(id, requestedAlias, 20, 7, account.mail_read_method || mailMethod, 1)
      .then((value) => {
        const normalized = normalizeInbox(value);
        rememberInboxCount(id, requestedAlias, normalized.count);
        if (request === inboxRequest.current) {
          setInbox(normalized);
          setInboxPage(1);
          setInboxCount(normalized.count);
          setSelectedMessages([]);
        }
      })
      .catch((e) => {
        if (request === inboxRequest.current) setNotice((e as Error).message);
      })
      .finally(() => {
        if (request === inboxRequest.current) setBusy(false);
      });
  }, [account?.id, account?.mail_read_method, id, requestedAlias, requestedTab]);
  useEffect(() => {
    if (requestedTab === "inbox") return;
    inboxRequest.current += 1;
    messageRequest.current += 1;
    setTab(requestedTab);
    setAlias("");
    setInbox(null);
    setMessage(null);
    setMessageError("");
    setMessageLoading(false);
    setAliasPage(requestedAliasPage);
  }, [requestedAliasPage, requestedTab]);
  const groupNames = useMemo(
    () => Object.fromEntries(groups.map((group) => [group.id, group.name])),
    [groups],
  );
  const activeAliases = useMemo(
    () => aliases.filter((item) => item.active),
    [aliases],
  );
  const disabledAliases = useMemo(
    () => aliases.filter((item) => !item.active),
    [aliases],
  );
  const visibleAliases = tab === "disabled" ? disabledAliases : activeAliases;
  const filteredAliases = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return visibleAliases.filter((item) => {
      const meta = metadata[item.anonymousId];
      const groupMatch =
        groupFilter === "all" ||
        (groupFilter === "ungrouped"
          ? !meta?.group_id
          : meta?.group_id === groupFilter);
      return (
        groupMatch &&
        (!needle ||
          item.email.toLowerCase().includes(needle) ||
          item.label.toLowerCase().includes(needle) ||
          meta?.note?.toLowerCase().includes(needle))
      );
    });
  }, [groupFilter, metadata, query, visibleAliases]);
  const totalPages = Math.max(1, Math.ceil(filteredAliases.length / PAGE_SIZE));
  const pagedAliases = useMemo(
    () =>
      filteredAliases.slice((aliasPage - 1) * PAGE_SIZE, aliasPage * PAGE_SIZE),
    [aliasPage, filteredAliases],
  );
  const aliasLoad = useMemo(
    () => ({ active: activeAliases.length, total: aliases.length }),
    [activeAliases.length, aliases.length],
  );
  const groupCounts = useMemo(() => {
    const counts: Record<string, number> = {
      all: visibleAliases.length,
      ungrouped: 0,
    };
    visibleAliases.forEach((item) => {
      const groupId = metadata[item.anonymousId]?.group_id;
      if (groupId) counts[groupId] = (counts[groupId] || 0) + 1;
      else counts.ungrouped += 1;
    });
    return counts;
  }, [metadata, visibleAliases]);
  const allGroupCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    aliases.forEach((item) => {
      const groupId = metadata[item.anonymousId]?.group_id;
      if (groupId) counts[groupId] = (counts[groupId] || 0) + 1;
    });
    return counts;
  }, [aliases, metadata]);
  const shareCounts = useMemo(
    () => ({
      all: shares.length,
      active: shares.filter((item) => !isShareExpired(item)).length,
      expired: shares.filter(isShareExpired).length,
    }),
    [shares],
  );
  const filteredShares = useMemo(() => {
    const needle = shareQuery.trim().toLowerCase();
    return shares.filter((item) => {
      const matchesStatus =
        shareFilter === "all" ||
        (shareFilter === "expired"
          ? isShareExpired(item)
          : !isShareExpired(item));
      const matchesQuery =
        !needle ||
        [item.alias, item.label || "", item.token].some((value) =>
          value.toLowerCase().includes(needle),
        );
      return matchesStatus && matchesQuery;
    });
  }, [shareFilter, shareQuery, shares]);
  const groupedShares = useMemo(() => {
    const groups = new Map<string, ShareLink[]>();
    filteredShares.forEach((item) =>
      groups.set(item.alias, [...(groups.get(item.alias) || []), item]),
    );
    return [...groups.entries()];
  }, [filteredShares]);
  const allDisabledSelected =
    filteredAliases.length > 0 &&
    filteredAliases.every((item) =>
      selectedDisabled.includes(item.anonymousId),
    );
  const allActiveSelected =
    filteredAliases.length > 0 &&
    filteredAliases.every((item) => selectedActive.includes(item.anonymousId));
  useEffect(() => {
    setSelectedActive((current) =>
      current.filter((selectedId) =>
        activeAliases.some((item) => item.anonymousId === selectedId),
      ),
    );
  }, [activeAliases]);
  useEffect(() => {
    setAliasPage(1);
  }, [groupFilter, query]);
  useEffect(() => {
    setSelectedActive([]);
    setSelectedDisabled([]);
  }, [groupFilter, query, tab]);
  useEffect(() => {
    setAliasPage((current) => Math.min(current, totalPages));
  }, [totalPages]);
  useEffect(() => {
    setSelectedDisabled((current) =>
      current.filter((selectedId) =>
        disabledAliases.some((item) => item.anonymousId === selectedId),
      ),
    );
  }, [disabledAliases]);
  useEffect(() => {
    setSelectedShares((current) =>
      current.filter((token) =>
        filteredShares.some((item) => item.token === token),
      ),
    );
  }, [filteredShares]);
  useEffect(() => {
    if (tab === "inbox" || requestedTab !== tab) return;
    const next = new URLSearchParams(searchParams);
    next.set("tab", tab);
    next.delete("alias");
    next.delete("from");
    next.delete("fromPage");
    if ((tab === "aliases" || tab === "disabled") && aliasPage > 1) {
      next.set("page", String(aliasPage));
    } else {
      next.delete("page");
    }
    if (next.toString() !== searchParams.toString()) {
      setSearchParams(next, { replace: true });
    }
  }, [aliasPage, requestedTab, searchParams, setSearchParams, tab]);
  useEffect(() => {
    if (!notice) return undefined;
    const timer = window.setTimeout(() => setNotice(""), 4200);
    return () => window.clearTimeout(timer);
  }, [notice]);

  const createAlias = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setBusy(true);
    try {
      const result = await api.createAlias(id, label.trim(), newAliasGroupId);
      setCreatedAlias(result.email);
      setNotice(
        `已创建 ${result.email}${newAliasGroupId ? `，已归入「${groupNames[newAliasGroupId] || "自定义分组"}」` : "，当前为未分组"}。`,
      );
      setLabel("");
      setNewAliasGroupId("");
      setAddOpen(false);
      await load();
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const batchLabelPreview = (offset: number) => {
    if (batchNamingRule === "same") return batchPrefix.trim() || "名称";
    return `${batchPrefix.trim() || "名称"}${batchSeparator}${String(batchStartNumber + offset).padStart(batchPadding, "0")}`;
  };
  const createAliasBatch = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setBusy(true);
    try {
      const result = await api.batchCreate(id, {
        count: batchCount,
        label_prefix: batchPrefix.trim(),
        naming_rule: batchNamingRule,
        separator: batchSeparator,
        start_number: batchStartNumber,
        padding: batchPadding,
        group_id: batchGroupId,
        note: batchNote.trim(),
      });
      setBatchResult(result);
      setBatchOpen(false);
      setNotice(`批量创建完成：成功 ${result.succeeded} 个，失败 ${result.failed} 个。`);
      await load();
    } catch (e) {
      setNotice(`批量创建失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const openInbox = async (
    selectedAlias = "",
    preferredMethod: MailReadPreference = mailMethod,
    requestedPage = 1,
    selectedDays: MailRangeDays = mailRangeDays,
  ) => {
    if (requestedTab !== "inbox" || requestedAlias !== selectedAlias) {
      const next = new URLSearchParams();
      next.set("tab", "inbox");
      if (selectedAlias) next.set("alias", selectedAlias);
      if (tab !== "inbox") {
        next.set("from", tab);
        if ((tab === "aliases" || tab === "disabled") && aliasPage > 1) {
          next.set("fromPage", String(aliasPage));
        }
      }
      setSearchParams(next);
      return;
    }
    const request = ++inboxRequest.current;
    messageRequest.current += 1;
    setMessage(null);
    setMessageError("");
    setMessageLoading(false);
    setInbox(null);
    setInboxCount(cachedInboxCount(id, selectedAlias));
    setAlias(selectedAlias);
    setInboxPage(requestedPage);
    setTab("inbox");
    setBusy(true);
    try {
      const normalized = normalizeInbox(
        await api.inbox(id, selectedAlias, 20, selectedDays, preferredMethod, requestedPage),
      );
      rememberInboxCount(id, selectedAlias, normalized.count);
      if (request === inboxRequest.current) {
        setInbox({ ...normalized, messages: normalized.messages.filter((item) => !deletingMessages.current.has(mailKey(item))) });
        setInboxCount(normalized.count);
        setSelectedMessages([]);
      }
    } catch (e) {
      if (request === inboxRequest.current) setNotice((e as Error).message);
    } finally {
      if (request === inboxRequest.current) setBusy(false);
    }
  };
  const openDetailTab = (nextTab: Exclude<Tab, "inbox">) => {
    const next = new URLSearchParams();
    next.set("tab", nextTab);
    setSearchParams(next);
  };
  const returnFromInbox = () => {
    const fromValue = searchParams.get("from");
    const fromTab: Exclude<Tab, "inbox"> =
      fromValue === "disabled" || fromValue === "shares"
        ? fromValue
        : "aliases";
    const fromPage = Math.max(
      1,
      Number.parseInt(searchParams.get("fromPage") || "1", 10) || 1,
    );
    const next = new URLSearchParams();
    next.set("tab", fromTab);
    if ((fromTab === "aliases" || fromTab === "disabled") && fromPage > 1) {
      next.set("page", String(fromPage));
    }
    setSearchParams(next, { replace: true });
  };
  const refreshInbox = async (
    selectedAlias = alias,
    preferredMethod: MailReadPreference = mailMethod,
    requestedPage = inboxPage,
    selectedDays: MailRangeDays = mailRangeDays,
    shouldApply: () => boolean = () => true,
  ) => {
    if (autoRefreshRunning.current) return;
    autoRefreshRunning.current = true;
    const request = ++inboxRequest.current;
    try {
      const normalized = normalizeInbox(
        await api.inbox(id, selectedAlias, 20, selectedDays, preferredMethod, requestedPage),
      );
      rememberInboxCount(id, selectedAlias, normalized.count);
      if (shouldApply() && request === inboxRequest.current) {
        setInbox({ ...normalized, messages: normalized.messages.filter((item) => !deletingMessages.current.has(mailKey(item))) });
        setInboxCount(normalized.count);
      }
    } catch (e) {
      if (shouldApply() && request === inboxRequest.current) setNotice(`自动刷新失败：${errorText(e)}`);
    } finally {
      autoRefreshRunning.current = false;
    }
  };
  useEffect(() => {
    localStorage.setItem("mail-auto-refresh-ms", String(autoRefreshMs));
    if (tab !== "inbox" || autoRefreshMs === 0) return undefined;
    let timer: number | undefined;
    let cancelled = false;
    const schedule = () => {
      timer = window.setTimeout(async () => {
        await refreshInbox(alias, mailMethod, inboxPage, mailRangeDays, () => !cancelled);
        if (!cancelled) schedule();
      }, autoRefreshMs);
    };
    schedule();
    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [alias, autoRefreshMs, inboxPage, mailMethod, mailRangeDays, tab]);
  const openMessage = async (item: MailMessage) => {
    const request = ++messageRequest.current;
    setMessage({ ...item, body: "", content_type: "" });
    setMessageError("");
    if (!inbox || inbox.method === "web_api") return;
    setMessageLoading(true);
    try {
      const result = await api.getMessage(
        id,
        item.id,
        item.folder,
        inbox.method,
        item.alias || alias,
      );
      if (request === messageRequest.current) setMessage(result);
    } catch (e) {
      if (request === messageRequest.current)
        setMessageError((e as Error).message);
    } finally {
      if (request === messageRequest.current) setMessageLoading(false);
    }
  };
  const closeMessage = () => {
    messageRequest.current += 1;
    setMessage(null);
    setMessageError("");
    setMessageLoading(false);
  };
  const deleteMessage = async () => {
    if (messageDeleting || !message || !inbox || inbox.method === "web_api") return;
    const target = message;
    const targetKey = mailKey(target);
    setMessageDeleting(true);
    deletingMessages.current.add(targetKey);
    setInbox((current) => current ? { ...current, messages: current.messages.filter((item) => mailKey(item) !== targetKey) } : current);
    setSelectedMessages((current) => current.filter((key) => key !== targetKey));
    setMessage(null);
    setMessageDeleteConfirm(false);
    setNotice("正在从邮箱服务器删除邮件…");
    try {
      await api.deleteMessage(id, target.id, target.folder, inbox.method, target.alias || alias);
      setNotice("邮件已删除。");
    } catch (e) {
      setInbox((current) => current ? { ...current, messages: newestFirst([...current.messages.filter((item) => mailKey(item) !== targetKey), target]) } : current);
      setNotice(`删除邮件失败：${errorText(e)}`);
    } finally {
      deletingMessages.current.delete(targetKey);
      setMessageDeleting(false);
    }
  };
  const toggleMessageSelection = (item: MailMessage) => {
    const key = mailKey(item);
    setSelectedMessages((current) =>
      current.includes(key)
        ? current.filter((itemKey) => itemKey !== key)
        : [...current, key],
    );
  };
  const deleteSelectedMessages = async () => {
    if (messageDeleting || !inbox || inbox.method === "web_api" || !selectedMessages.length)
      return;
    const targets = inbox.messages.filter((item) =>
      selectedMessages.includes(mailKey(item)),
    );
    const targetKeys = new Set(targets.map(mailKey));
    setMessageDeleting(true);
    targetKeys.forEach((key) => deletingMessages.current.add(key));
    setInbox((current) => current ? { ...current, messages: current.messages.filter((item) => !targetKeys.has(mailKey(item))) } : current);
    setSelectedMessages([]);
    setMessageBatchDeleteConfirm(false);
    setNotice(`正在从邮箱服务器删除 ${targets.length} 封邮件…`);
    try {
	  const result = await api.deleteMessages(
		id,
		inbox.method,
		targets.map((item) => ({ uid: item.id, folder: item.folder, alias: item.alias || alias })),
	  );
	  setNotice(`已删除 ${result.deleted} 封邮件。`);
	} catch (e) {
	  setInbox((current) => current ? { ...current, messages: newestFirst([...current.messages, ...targets]) } : current);
	  setNotice(`批量删除邮件失败：${errorText(e)}`);
	} finally {
      targetKeys.forEach((key) => deletingMessages.current.delete(key));
      setMessageDeleting(false);
    }
  };
  const toggleAlias = async (item: Alias) => {
    setBusy(true);
    try {
      if (item.active) {
        await api.deactivateAlias(id, item.anonymousId);
        setAliases((current) =>
          current.map((aliasItem) =>
            aliasItem.anonymousId === item.anonymousId
              ? { ...aliasItem, active: false }
              : aliasItem,
          ),
        );
        setNotice(`${item.email} 已停用，可在停用邮箱中恢复。`);
      } else {
        await api.reactivateAlias(id, item.anonymousId);
        setAliases((current) =>
          current.map((aliasItem) =>
            aliasItem.anonymousId === item.anonymousId
              ? { ...aliasItem, active: true }
              : aliasItem,
          ),
        );
        setNotice(`${item.email} 已恢复。`);
      }
    } catch (e) {
      setNotice(
        `${item.active ? "停用" : "恢复"} ${item.email} 失败：${errorText(e)}`,
      );
    } finally {
      setBusy(false);
    }
  };
  const toggleDisabledSelection = (aliasId: string) =>
    setSelectedDisabled((current) =>
      current.includes(aliasId)
        ? current.filter((item) => item !== aliasId)
        : [...current, aliasId],
    );
  const toggleActiveSelection = (aliasId: string) =>
    setSelectedActive((current) =>
      current.includes(aliasId)
        ? current.filter((item) => item !== aliasId)
        : [...current, aliasId],
    );
  const toggleAllActive = () =>
    setSelectedActive(
      allActiveSelected
        ? []
        : filteredAliases.map((item) => item.anonymousId),
    );
  const disableSelectedActive = async () => {
    if (!selectedActive.length) return;
    setBusy(true);
    try {
      const targets = [...selectedActive];
      const results = await Promise.allSettled(
        targets.map((aliasId) => api.deactivateAlias(id, aliasId)),
      );
      const disabled = new Set(
        targets.filter((_, index) => results[index].status === "fulfilled"),
      );
      setAliases((current) =>
        current.map((item) =>
          disabled.has(item.anonymousId) ? { ...item, active: false } : item,
        ),
      );
      setSelectedActive((current) => current.filter((item) => !disabled.has(item)));
      setActiveDeleteConfirm(false);
      const failed = targets.length - disabled.size;
      setNotice(
        `已停用 ${disabled.size} 个邮箱${failed ? `，${failed} 个失败` : ""}，可在停用邮箱中恢复。`,
      );
    } catch (e) {
      setNotice(`批量停用邮箱失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const toggleAllDisabled = () =>
    setSelectedDisabled(
      allDisabledSelected
        ? []
        : filteredAliases.map((item) => item.anonymousId),
    );
  const restoreDisabled = async (item: Alias) => {
    setBusy(true);
    try {
      await api.reactivateAlias(id, item.anonymousId);
      setAliases((current) =>
        current.map((aliasItem) =>
          aliasItem.anonymousId === item.anonymousId
            ? { ...aliasItem, active: true }
            : aliasItem,
        ),
      );
      setNotice(`${item.email} 已恢复。`);
    } catch (e) {
      setNotice(`恢复 ${item.email} 失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const restoreSelectedDisabled = async () => {
    if (!selectedDisabled.length) return;
    setBusy(true);
    const ids = new Set(selectedDisabled);
    const results = await Promise.allSettled(
      selectedDisabled.map((aliasId) => api.reactivateAlias(id, aliasId)),
    );
    const succeeded = results.filter(
      (result) => result.status === "fulfilled",
    ).length;
    const failures = results
      .filter(
        (result): result is PromiseRejectedResult =>
          result.status === "rejected",
      )
      .map((result) => errorText(result.reason));
    setAliases((current) =>
      current.map((aliasItem) =>
        ids.has(aliasItem.anonymousId) &&
        results[selectedDisabled.indexOf(aliasItem.anonymousId)]?.status ===
          "fulfilled"
          ? { ...aliasItem, active: true }
          : aliasItem,
      ),
    );
    setNotice(
      `已恢复 ${succeeded} 个停用别名${failures.length ? `；失败详情：${failures.join("；")}` : ""}。`,
    );
    setSelectedDisabled([]);
    setBusy(false);
  };
  const deleteSelectedDisabled = async () => {
    if (!selectedDisabled.length) return;
    setBusy(true);
    const ids = new Set(selectedDisabled);
    const results = await Promise.allSettled(
      selectedDisabled.map((aliasId) => api.deleteAlias(id, aliasId)),
    );
    const succeeded = results.filter(
      (result) => result.status === "fulfilled",
    ).length;
    const failed = results.length - succeeded;
    setAliases((current) =>
      current.filter(
        (aliasItem) =>
          !ids.has(aliasItem.anonymousId) ||
          results[selectedDisabled.indexOf(aliasItem.anonymousId)]?.status !==
            "fulfilled",
      ),
    );
    setNotice(
      `已删除 ${succeeded} 个停用别名${failed ? `，${failed} 个失败` : ""}。`,
    );
    setSelectedDisabled([]);
    setDisabledDeleteConfirm(false);
    setBusy(false);
  };
  const deleteAlias = async () => {
    if (!deleteTarget) return;
    setBusy(true);
    try {
      await api.deleteAlias(id, deleteTarget.anonymousId);
      setAliases((current) =>
        current.filter(
          (aliasItem) => aliasItem.anonymousId !== deleteTarget.anonymousId,
        ),
      );
      setNotice(`已删除 ${deleteTarget.email}。`);
      setDeleteTarget(null);
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const openAliasEditor = (item: Alias) => {
    const current = metadata[item.anonymousId];
    setEditorLabel(item.label || current?.label || "");
    setEditorGroupId(current?.group_id || "");
    setEditorNote(current?.note || "");
    setEditorAlias(item);
  };
  const saveAliasMeta = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!editorAlias) return;
    setBusy(true);
    const nextMeta: AliasMetadata = {
      alias_id: editorAlias.anonymousId,
      email: editorAlias.email,
      label: editorLabel.trim(),
      group_id: editorGroupId,
      note: editorNote,
      updated_at: new Date().toISOString(),
    };
    try {
      await api.updateAliasMeta(id, nextMeta);
      setMetadata((current) => ({
        ...current,
        [editorAlias.anonymousId]: nextMeta,
      }));
      setAliases((current) =>
        current.map((item) =>
          item.anonymousId === editorAlias.anonymousId
            ? { ...item, label: nextMeta.label }
            : item,
        ),
      );
      setEditorAlias(null);
      setNotice("别名名称已同步到 iCloud，分组和备注已保存到本项目。");
    } catch (e) {
      setNotice(`保存 ${editorAlias.email} 的归类与备注失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const createGroup = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const name = groupName.trim();
    if (!name) return;
    setBusy(true);
    try {
      const group = await api.createOrganizerGroup(id, name);
      setGroups((current) => [...current, group]);
      setGroupName("");
      setNotice("本地分组已创建。");
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const renameGroup = async (group: OrganizerGroup) => {
    const name = renameValue.trim();
    if (!name) return;
    setBusy(true);
    try {
      await api.renameOrganizerGroup(id, group.id, name);
      setGroups((current) =>
        current.map((item) =>
          item.id === group.id ? { ...item, name } : item,
        ),
      );
      setRenameGroupId(null);
      setNotice("本地分组已重命名。");
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const deleteGroup = async () => {
    if (!groupDeleteTarget) return;
    setBusy(true);
    try {
      await api.deleteOrganizerGroup(id, groupDeleteTarget.id);
      setGroups((current) =>
        current.filter((item) => item.id !== groupDeleteTarget.id),
      );
      setMetadata((current) =>
        Object.fromEntries(
          Object.entries(current).map(([key, value]) => [
            key,
            value.group_id === groupDeleteTarget.id
              ? { ...value, group_id: "" }
              : value,
          ]),
        ),
      );
      setGroupDeleteTarget(null);
      setNotice("分组已删除，别名仍保留为未分组。");
    } catch (e) {
      setNotice((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const moveGroup = (sourceId: string, targetId: string, placeAfter: boolean) => {
    if (sourceId === targetId) return;
    const currentSourceIndex = groups.findIndex((item) => item.id === sourceId);
    const currentTargetIndex = groups.findIndex((item) => item.id === targetId);
    if (currentSourceIndex < 0 || currentTargetIndex < 0) return;
    if (currentSourceIndex < currentTargetIndex && !placeAfter) return;
    if (currentSourceIndex > currentTargetIndex && placeAfter) return;
    const previousRects = new Map<string, DOMRect>();
    groupRowRefs.current.forEach((row, groupId) => {
      previousRects.set(groupId, row.getBoundingClientRect());
    });
    groupMoveAnimations.current.forEach((animation) => animation.cancel());
    groupMoveAnimations.current.clear();
    pendingGroupRects.current = previousRects;
    setGroups((current) => {
      const sourceIndex = current.findIndex((item) => item.id === sourceId);
      const targetIndex = current.findIndex((item) => item.id === targetId);
      if (sourceIndex < 0 || targetIndex < 0) return current;
      const next = [...current];
      const [moved] = next.splice(sourceIndex, 1);
      const nextTargetIndex = next.findIndex((item) => item.id === targetId);
      next.splice(nextTargetIndex + (placeAfter ? 1 : 0), 0, moved);
      return next;
    });
    setGroupOrderDirty(true);
  };
  const moveGroupDrag = (pointerId: number, clientX: number, clientY: number) => {
    const drag = groupDrag.current;
    if (!drag || drag.pointerId !== pointerId) return;
    drag.pointerY = clientY;
    const sourceRow = groupRowRefs.current.get(drag.sourceId);
    if (sourceRow) {
      const current = sourceRow.getBoundingClientRect();
      const layoutTop = current.top - drag.translateY;
      drag.translateY = clientY - drag.grabOffsetY - layoutTop;
      sourceRow.style.transform = `translate3d(0, ${drag.translateY}px, 0)`;
    }
    if (Math.abs(clientY - drag.startY) < 5) return;
    const targetRow = document
      .elementsFromPoint(clientX, clientY)
      .map((element) => element.closest<HTMLElement>("[data-group-id]"))
      .find((row) => row?.dataset.groupId && row.dataset.groupId !== drag.sourceId);
    const target = targetRow?.dataset.groupId;
    if (target && targetRow) {
      setDragOverGroupId(target);
      const box = targetRow.getBoundingClientRect();
      moveGroup(drag.sourceId, target, clientY >= box.top + box.height / 2);
    }
  };
  const finishGroupDrag = () => {
    const drag = groupDrag.current;
    if (!drag) return;
    groupDrag.current = null;
    const row = groupRowRefs.current.get(drag.sourceId);
    if (row) {
      row.style.transform = "";
      if (
        Math.abs(drag.translateY) >= 1 &&
        !window.matchMedia("(prefers-reduced-motion: reduce)").matches
      ) {
        const animation = row.animate(
          [
            { transform: `translate3d(0, ${drag.translateY}px, 0)` },
            { transform: "translate3d(0, 0, 0)" },
          ],
          { duration: 150, easing: "cubic-bezier(.2, .8, .2, 1)" },
        );
        groupMoveAnimations.current.set(drag.sourceId, animation);
      }
    }
    setDraggedGroupId(null);
    setDragOverGroupId(null);
  };
  useLayoutEffect(() => {
    if (pendingGroupRects.current.size === 0) return;
    const previousRects = pendingGroupRects.current;
    pendingGroupRects.current = new Map();
    const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    groupRowRefs.current.forEach((row, groupId) => {
      const activeDrag = groupDrag.current;
      if (activeDrag?.sourceId === groupId) {
        const current = row.getBoundingClientRect();
        const desiredTop = activeDrag.pointerY - activeDrag.grabOffsetY;
        activeDrag.translateY += desiredTop - current.top;
        row.style.transform = `translate3d(0, ${activeDrag.translateY}px, 0)`;
        return;
      }
      if (reducedMotion) return;
      const previous = previousRects.get(groupId);
      if (!previous) return;
      const current = row.getBoundingClientRect();
      const offsetY = previous.top - current.top;
      if (Math.abs(offsetY) < 1) return;
      const animation = row.animate(
        [
          { transform: `translate3d(0, ${offsetY}px, 0)` },
          { transform: "translate3d(0, 0, 0)" },
        ],
        {
          duration: 190,
          easing: "cubic-bezier(.2, .8, .2, 1)",
        },
      );
      groupMoveAnimations.current.set(groupId, animation);
      animation.addEventListener("finish", () => {
        if (groupMoveAnimations.current.get(groupId) === animation) {
          groupMoveAnimations.current.delete(groupId);
        }
      });
    });
  }, [groups]);
  const saveGroupOrder = async () => {
    if (!groupOrderDirty) return;
    setBusy(true);
    try {
      await api.reorderOrganizerGroups(id, groups.map((group) => group.id));
      setGroupOrderDirty(false);
      setNotice("分组排序已保存。");
    } catch (e) {
      setNotice(`保存分组排序失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const openSharePanel = (email?: string) => {
    setShareAlias(email || activeAliases[0]?.email || "");
    setShareLabel("");
    setShareDuration("0");
    setShareOpen(true);
  };
  const createShare = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!shareAlias || shareCreating) return;
    const expiresMinutes = parseDurationMinutes(shareDuration);
    if (expiresMinutes === null) {
      setNotice("请输入有效时长，例如 2h、14d、1h30m；输入 0 表示永久。");
      return;
    }
    setShareCreating(true);
    try {
      const result = await api.createShare(
        id,
        shareAlias,
        shareLabel.trim(),
        expiresMinutes,
      );
      const path = result.url || `/share/${result.token}`;
      setShareOpen(false);
      setShareResult(new URL(path, window.location.origin).toString());
      setNotice("分享链接已创建。");
      setShares(await api.listShares(id));
    } catch (e) {
      setNotice(`创建分享链接失败：${errorText(e)}`);
    } finally {
      setShareCreating(false);
    }
  };
  const copyShare = async (item: ShareLink) => {
    await navigator.clipboard.writeText(
      `${window.location.origin}/share/${item.token}`,
    );
    setNotice("分享链接已复制。");
  };
  const openShareEditor = (item: ShareLink) => {
    setShareEditTarget(item);
    setShareEditLabel(item.label || "");
  };
  const saveShareLabel = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!shareEditTarget) return;
    setBusy(true);
    try {
      const updated = await api.updateShare(shareEditTarget.token, shareEditLabel);
      setShares((current) => current.map((item) => item.token === updated.token ? updated : item));
      setShareEditTarget(null);
      setNotice("分享链接备注已保存。");
    } catch (e) {
      setNotice(`保存分享链接备注失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const toggleShareSelection = (token: string) =>
    setSelectedShares((current) =>
      current.includes(token)
        ? current.filter((item) => item !== token)
        : [...current, token],
    );
  const toggleAllShares = () =>
    setSelectedShares(
      selectedShares.length === filteredShares.length
        ? []
        : filteredShares.map((item) => item.token),
    );
  const deleteSelectedShares = async () => {
    if (!selectedShares.length) return;
    setBusy(true);
    try {
      const result = await api.batchDeleteShares(selectedShares);
      const deleted = new Set(selectedShares);
      setShares((current) =>
        current.filter((item) => !deleted.has(item.token)),
      );
      setSelectedShares([]);
      setShareDeleteConfirm(false);
      setNotice(
        `已删除 ${result.deleted} 个分享链接${result.not_found ? `，${result.not_found} 个不存在` : ""}。`,
      );
    } catch (e) {
      setNotice(`批量删除分享链接失败：${errorText(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const disableSelectedShareAliases = async () => {
    const ids = [
      ...new Set(
        selectedShares
          .map((token) => shares.find((share) => share.token === token)?.alias)
          .map(
            (email) =>
              aliases.find((item) => item.email === email)?.anonymousId,
          )
          .filter((value): value is string => Boolean(value)),
      ),
    ];
    if (!ids.length) return;
    if (
      !window.confirm(
        `确认停用 ${ids.length} 个邮箱吗？停用后可在停用邮箱中恢复。`,
      )
    )
      return;
    setBusy(true);
    const results = await Promise.allSettled(
      ids.map((aliasId) => api.deactivateAlias(id, aliasId)),
    );
    const succeeded = results.filter(
      (result) => result.status === "fulfilled",
    ).length;
    const done = new Set(
      ids.filter((_, index) => results[index]?.status === "fulfilled"),
    );
    setAliases((current) =>
      current.map((item) =>
        done.has(item.anonymousId) ? { ...item, active: false } : item,
      ),
    );
    setSelectedShares([]);
    setNotice(
      `已停用 ${succeeded} 个邮箱${succeeded < ids.length ? `，${ids.length - succeeded} 个失败` : ""}。`,
    );
    setBusy(false);
  };
  const refreshAfterPassword = (messageText: string) => {
    setNotice(messageText);
    void load().then(async () => {
      if (tab === "inbox")
        setInbox(normalizeInbox(await api.inbox(id, alias, 20, mailRangeDays, mailMethod, inboxPage)));
    });
  };
  const renderAliasRow = (item: Alias, index: number) => {
    const meta = metadata[item.anonymousId];
    const disabled = !item.active;
    return (
      <div
        className="alias-row"
        key={item.anonymousId}
        style={{ animationDelay: `${index * 35}ms` }}
      >
        <label className="checkbox-hit alias-select">
          <input
            type="checkbox"
            aria-label={`选择 ${item.email}`}
            checked={
              disabled
                ? selectedDisabled.includes(item.anonymousId)
                : selectedActive.includes(item.anonymousId)
            }
            onChange={() =>
              disabled
                ? toggleDisabledSelection(item.anonymousId)
                : toggleActiveSelection(item.anonymousId)
            }
          />
        </label>
        <span className={`alias-state ${item.active ? "on" : "off"}`} />
        <div className="alias-main">
          <strong className="mono">{item.email}</strong>
          <span>
            {item.label || "未命名"}
            {item.forwardTo ? ` · 转发至 ${item.forwardTo}` : ""}
          </span>
          <small className="alias-created-at">
            {parseDateValue(item.createdAt)
              ? `创建 ${compactDateText(item.createdAt)}`
              : "时间未知"}
          </small>
          <small className="alias-local-summary">
            {meta?.group_id
              ? `分组：${groupNames[meta.group_id] || "未知分组"}`
              : "未分组"}
            {meta?.note ? ` · 备注：${meta.note}` : ""}
          </small>
        </div>
        {disabled && <span className="status status-disabled">已禁用</span>}
        <div className="alias-actions">
          <button
            className="button small secondary"
            onClick={() => openAliasEditor(item)}
            aria-label={`编辑 ${item.email}`}
            title="编辑名称、分组和备注"
          >
            <Icon name="settings" size={15} />
            <span className="alias-action-label">编辑</span>
          </button>
          {!disabled && (
            <>
              <button
                className="button small secondary"
                onClick={() => openInbox(item.email)}
                aria-label={`查看 ${item.email} 的收件箱`}
                title="收件箱"
              >
                <Icon name="mail" size={15} />
                <span className="alias-action-label">收件箱</span>
              </button>
              <button
                className="button small secondary"
                aria-label={`复制 ${item.email}`}
                title="复制邮箱"
                onClick={() => {
                  navigator.clipboard.writeText(item.email);
                  setNotice("别名已复制。");
                }}
              >
                <Icon name="copy" size={15} />
                <span className="alias-action-label">复制</span>
              </button>
              <button
                className="button small secondary"
                onClick={() => openSharePanel(item.email)}
                aria-label={`分享 ${item.email}`}
                title="创建分享链接"
              >
                <Icon name="link" size={15} />
                <span className="alias-action-label">分享</span>
              </button>
              <button
                className="button small alias-disable-action"
                onClick={() => toggleAlias(item)}
                aria-label={`停用 ${item.email}`}
                title="停用邮箱"
              >
                <Icon name="archive" size={15} />
                <span className="alias-action-label">停用</span>
              </button>
            </>
          )}
          {disabled && (
            <>
              <button
                className="button small secondary"
                onClick={() => restoreDisabled(item)}
                aria-label={`恢复 ${item.email}`}
                title="恢复邮箱"
              >
                <Icon name="restore" size={15} />
                <span className="alias-action-label">恢复</span>
              </button>
              <button
                className="button small alias-delete-action"
                onClick={() => setDeleteTarget(item)}
                aria-label={`删除 ${item.email}`}
                title="删除邮箱"
              >
                <Icon name="trash" size={15} />
                <span className="alias-action-label">删除</span>
              </button>
            </>
          )}
        </div>
      </div>
    );
  };
  if (!account && !busy)
    return (
      <PageLayout title="账号详情" showHeading={false} onLogout={onLogout}>
        <div className="empty-state panel">
          <h3>该账号不可用。</h3>
          <button className="button secondary" onClick={() => navigate("/")}>
            <Icon name="back" size={16} />
            返回账号列表
          </button>
        </div>
      </PageLayout>
    );

  return (
    <PageLayout
      title="账号详情"
      eyebrow="账号详情"
      showHeading={false}
      onLogout={onLogout}
    >
      {notice && (
        <div className="notice toast" role="status" aria-live="polite">
          <span>{notice}</span>
          <button className="text-button" onClick={() => setNotice("")}>
            关闭
          </button>
        </div>
      )}
      <section className="detail-hero panel">
        <button
          className="back-link"
          onClick={() => {
            if (tab === "inbox") {
              returnFromInbox();
              return;
            }
            navigate(account?.status === "disabled" ? "/disabled" : "/");
          }}
          aria-label={tab === "inbox" ? "返回邮箱列表" : "返回账号列表"}
        >
          <Icon name="back" size={16} />
        </button>
        <div className="detail-identity">
          <div className="detail-account-copy">
            <div className="detail-account-title-row">
              <h1>{account?.name}</h1>
              <span
                className={`status status-${account?.status === "active" ? "ready" : account?.status === "disabled" ? "disabled" : "pending"}`}
              >
                {account?.status === "active"
                  ? "正常"
                  : account?.status === "disabled"
                    ? "已禁用"
                    : "待处理"}
              </span>
            </div>
            <div className="detail-account-meta-row">
              <span className="mono detail-email">
                {account?.real_email || account?.icloud_email || account?.id}
              </span>
              <strong className="alias-count">
                {aliasLoad.active} / {aliasLoad.total} 个别名
              </strong>
            </div>
          </div>
        </div>
        <div className="detail-state">
          <span className="detail-meta">{account?.host || "icloud.com"}</span>
          <button
            className="button small secondary mail-config-trigger"
            onClick={() => setMailConfigOpen(true)}
          >
            <Icon name="settings" size={15} />
            邮件读取配置
          </button>
        </div>
      </section>
      {mailMethodNotice && <div className="inline-banner">{mailMethodNotice}</div>}
      <section className="account-health" aria-label="账号连接状态">
        <div>
          <strong>iCloud 会话</strong>
          <span
            className={`health-value ${account?.status === "active" ? "ready" : "pending"}`}
          >
            {account?.status === "active"
              ? "可用"
              : account?.status === "error"
                ? "需要检查"
                : "待验证"}
          </span>
        </div>
        <div>
          <strong>邮件读取</strong>
          <span
            className={`health-value ${account?.has_forward_imap || account?.has_app_password ? "ready" : "pending"}`}
          >
            {account?.has_forward_imap
              ? `转发 IMAP · ${account.forward_imap?.email || "已配置"}`
              : account?.has_app_password
                ? "iCloud IMAP 已配置"
                : "Web API 摘要模式"}
          </span>
        </div>
        <div>
          <strong>最近验证</strong>
          <span>
            {account?.last_validated
              ? dateText(account.last_validated)
              : "尚未验证"}
          </span>
        </div>
        {account?.last_error && (
          <p className="health-error">{account.last_error}</p>
        )}
      </section>
      <section className="panel detail-panel">
        <div className="tabs" role="tablist">
          <button
            className={tab === "aliases" ? "active" : ""}
            onClick={() => openDetailTab("aliases")}
            role="tab"
            aria-selected={tab === "aliases"}
          >
            <Icon name="link" size={16} />
            活跃别名 <span>{activeAliases.length}</span>
          </button>
          <button
            className={tab === "disabled" ? "active" : ""}
            onClick={() => openDetailTab("disabled")}
            role="tab"
            aria-selected={tab === "disabled"}
          >
            <Icon name="archive" size={16} />
            停用邮箱 <span>{disabledAliases.length}</span>
          </button>
          <button
            className={tab === "inbox" ? "active" : ""}
            onClick={() => openInbox("")}
            role="tab"
            aria-selected={tab === "inbox"}
          >
            <Icon name="mail" size={16} />
            收件箱 <span>{inboxCount ?? "—"}</span>
          </button>
          <button
            className={tab === "shares" ? "active" : ""}
            onClick={() => openDetailTab("shares")}
            role="tab"
            aria-selected={tab === "shares"}
          >
            <Icon name="copy" size={16} />
            分享 <span>{shares.length}</span>
          </button>
        </div>
        {(tab === "aliases" || tab === "disabled") && (
          <>
            <div className="panel-header compact">
              <div>
                <span className="eyebrow">
                  {tab === "disabled" ? "已停用地址" : "正在使用"}
                </span>
                <h2>{tab === "disabled" ? "停用邮箱" : "别名清单"}</h2>
              </div>
              <div className="inline-actions alias-toolbar">
                <GroupFilter
                  groups={groups}
                  value={groupFilter}
                  counts={groupCounts}
                  onChange={setGroupFilter}
                />
                <button
                  className="button secondary"
                  onClick={() => setOrganizerOpen(true)}
                >
                  <Icon name="settings" size={16} />
                  管理分组
                </button>
                <label className="search-field compact-search">
                  <Icon name="search" size={16} />
                  <input
                    aria-label={
                      tab === "disabled" ? "搜索停用邮箱" : "搜索活跃别名"
                    }
                    placeholder={
                      tab === "disabled" ? "搜索停用邮箱" : "搜索别名"
                    }
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                  />
                </label>
                {tab === "aliases" && (
                  <>
                    <button className="button secondary" onClick={() => setBatchOpen(true)}>
                      批量创建
                    </button>
                    <button className="button primary" onClick={() => setAddOpen(true)}>
                      新建别名
                    </button>
                  </>
                )}
              </div>
            </div>
            <div className="organizer-caption">
              分组和备注仅保存在本地，用于整理，不会同步到 iCloud。
            </div>
            {tab === "disabled" && (
              <div className="disabled-alias-toolbar">
                <label className="checkbox-hit">
                  <input
                    type="checkbox"
                    aria-label="全选停用别名"
                    checked={allDisabledSelected}
                    onChange={toggleAllDisabled}
                  />
                </label>
                <span>
                  {selectedDisabled.length
                    ? `已选 ${selectedDisabled.length} 个`
                    : query || groupFilter !== "all"
                      ? `选择筛选结果（${filteredAliases.length}）`
                      : `选择全部（${filteredAliases.length}）`}
                </span>
                {selectedDisabled.length > 0 && (
                  <>
                    <button
                      className="button secondary"
                      onClick={restoreSelectedDisabled}
                    >
                      <Icon name="restore" size={15} />
                      批量恢复
                    </button>
                    <button
                      className="danger-ghost"
                      onClick={() => setDisabledDeleteConfirm(true)}
                    >
                      <Icon name="trash" size={15} />
                      批量删除
                    </button>
                    <button
                      className="text-button"
                      onClick={() => setSelectedDisabled([])}
                    >
                      清除选择
                    </button>
                  </>
                )}
              </div>
            )}
            {tab === "aliases" && (
              <div className="disabled-alias-toolbar active-alias-toolbar">
                <label className="checkbox-hit">
                  <input
                    type="checkbox"
                    aria-label="全选活跃别名"
                    checked={allActiveSelected}
                    onChange={toggleAllActive}
                  />
                </label>
                <span>{selectedActive.length ? `已选 ${selectedActive.length} 个` : query || groupFilter !== "all" ? `选择筛选结果（${filteredAliases.length}）` : `选择全部（${filteredAliases.length}）`}</span>
                {selectedActive.length > 0 && (
                  <>
                    <button className="button secondary" onClick={() => setActiveDeleteConfirm(true)}>
                      <Icon name="archive" size={15} />批量停用
                    </button>
                    <button className="text-button" onClick={() => setSelectedActive([])}>清除选择</button>
                  </>
                )}
              </div>
            )}
            <div className="alias-list">
              {aliasesLoading ? (
                <div className="alias-skeleton-list" role="status">
                  <span className="sr-only">正在加载邮箱</span>
                  {[1, 2, 3, 4, 5].map((item) => (
                    <div className="alias-skeleton-row" key={item}>
                      <span />
                      <span />
                      <span />
                    </div>
                  ))}
                </div>
              ) : filteredAliases.length === 0 ? (
                <div className="empty-state small-empty">
                  <h3>
                    {query || groupFilter !== "all"
                      ? "没有匹配的别名。"
                      : tab === "disabled"
                        ? "没有停用邮箱。"
                        : "还没有活跃别名。"}
                  </h3>
                  <p>
                    {query || groupFilter !== "all"
                      ? "请尝试其他筛选条件。"
                      : tab === "disabled"
                        ? "停用的别名会保留在这里，可随时恢复。"
                        : "创建一个私密地址即可开始。"}
                  </p>
                </div>
              ) : (
                pagedAliases.map(renderAliasRow)
              )}
            </div>
            {!aliasesLoading && totalPages > 1 && (
              <Pagination
                page={aliasPage}
                totalPages={totalPages}
                onChange={(page) => setAliasPage(page)}
                label="别名分页"
              />
            )}
          </>
        )}
        {tab === "inbox" && (
          <div className="inbox-view">
            <div className="panel-header compact">
              <div>
                <span className="eyebrow">最近收件</span>
                <h2>{alias || "全部邮件"}</h2>
              </div>
              <div className="inline-actions inbox-refresh-controls">
                <label className="search-field inbox-list-search">
                  <Icon name="search" size={16} />
                  <input
                    value={inboxQuery}
                    onChange={(event) => {
                      setInboxQuery(event.target.value);
                      setSelectedMessages([]);
                    }}
                    placeholder="搜索当前列表"
                    aria-label="搜索当前邮件列表"
                  />
                </label>
                <SelectMenu
                  value={String(mailRangeDays)}
                  className="auto-refresh-select"
                  ariaLabel="邮件时间范围"
                  options={[
                    { value: "7", label: "范围：近 7 天" },
                    { value: "0", label: "范围：全部" },
                  ]}
                  onChange={(value) => {
                    const next = Number(value) as MailRangeDays;
                    setMailRangeDays(next);
                    void openInbox(alias, mailMethod, 1, next);
                  }}
                />
                <SelectMenu
                  value={String(autoRefreshMs)}
                  className="auto-refresh-select"
                  ariaLabel="自动刷新间隔"
                  options={[
                    { value: "0", label: "自动刷新：关闭" },
                    { value: "5000", label: "自动刷新：5 秒" },
                    { value: "15000", label: "自动刷新：15 秒" },
                    { value: "30000", label: "自动刷新：30 秒" },
                  ]}
                  onChange={(value) => setAutoRefreshMs(Number(value) as RefreshInterval)}
                />
                <button
                  className="button secondary"
                  onClick={() => openInbox(alias, mailMethod, inboxPage)}
                >
                  <Icon name="refresh" size={16} />
                  刷新
                </button>
              </div>
            </div>
            {inbox?.method && (
              <div className="inline-banner mail-source-banner">
                <strong>当前读取方式：{mailMethodLabel(inbox.method)}</strong>
                <span>
                  {inbox.method === "web_api"
                    ? "当前显示主题、发件人和摘要；切换到 IMAP 后可查看完整正文。"
                    : "列表和点击后的完整正文都来自当前 IMAP 邮箱。"}
                </span>
              </div>
            )}
            {!inbox || !inbox.messages?.length ? (
              <div className="empty-state small-empty">
                <h3>{busy ? "正在读取邮件…" : "这段时间没有新邮件。"}</h3>
                <p>
                  {busy
                    ? "仅加载标题、发件人与时间，请稍候。"
                    : "最近 7 天收到的邮件会显示在这里。"}
                </p>
              </div>
            ) : (
              <div className="inbox-split">
                <div className="mail-list inbox-mail-list">
                  {inbox.method !== "web_api" && (
                    <div className="mail-batch-toolbar">
                      <label className="mail-select-all">
                        <input
                          type="checkbox"
                          checked={
                            visibleInboxMessages.length > 0 &&
                            visibleInboxMessages.every((item) =>
                              selectedMessages.includes(mailKey(item)),
                            )
                          }
                          onChange={(event) =>
                            setSelectedMessages(
                              event.target.checked
                                ? visibleInboxMessages.map(mailKey)
                                : [],
                            )
                          }
                        />
                        <span>
                          {selectedMessages.length
                            ? `已选 ${selectedMessages.length} 封`
                            : "选择邮件"}
                        </span>
                      </label>
                      {selectedMessages.length > 0 && (
                        <>
                          <button
                            className="text-button danger-text"
                            type="button"
                            onClick={() => setMessageBatchDeleteConfirm(true)}
                          >
                            <Icon name="trash" size={15} />
                            批量删除
                          </button>
                          <button
                            className="text-button"
                            type="button"
                            onClick={() => setSelectedMessages([])}
                          >
                            清除选择
                          </button>
                        </>
                      )}
                    </div>
                  )}
                  {visibleInboxMessages.length === 0 && inboxQuery.trim() ? (
                    <div className="empty-state small-empty inbox-search-empty">
                      <h3>当前列表没有匹配邮件。</h3>
                      <p>只搜索主题、发件人、收件人、摘要、验证码和时间，不搜索邮件正文。</p>
                    </div>
                  ) : visibleInboxMessages.map((item) => (
                    <InlineMailRow
                      item={item}
                      selected={message}
                      checked={selectedMessages.includes(mailKey(item))}
                      loading={messageLoading}
                      error={messageError}
                      method={inbox.method}
                      onOpen={() => void openMessage(item)}
                      onClose={closeMessage}
                      onRetry={() => void openMessage(item)}
                      onCopyCode={(code) => void copyCode(code, setNotice)}
                      onDelete={() => setMessageDeleteConfirm(true)}
                      onToggle={() => toggleMessageSelection(item)}
                      key={`${item.folder}-${item.id}`}
                    />
                  ))}
                </div>
              </div>
            )}
            {inbox && (inboxPage > 1 || inbox.has_more) && (
              <Pagination
                page={inboxPage}
                totalPages={inboxPage + (inbox.has_more ? 1 : 0)}
                onChange={(page) => void openInbox(alias, mailMethod, page)}
                label="邮件分页"
              />
            )}
          </div>
        )}
        {tab === "shares" && (
          <div>
            <div className="panel-header compact">
              <div>
                <span className="eyebrow">只读访问</span>
                <h2>
                  分享链接{" "}
                  <span className="count-badge">
                    {filteredShares.length}/{shares.length}
                  </span>
                </h2>
              </div>
              <button
                className="button primary"
                onClick={() => openSharePanel()}
                disabled={activeAliases.length === 0}
              >
                创建分享链接
              </button>
            </div>
            <div className="share-toolbar">
              <label className="search-field share-search">
                <Icon name="search" size={16} />
                <input
                  aria-label="搜索分享链接"
                  placeholder="搜索邮箱、标签或令牌"
                  value={shareQuery}
                  onChange={(event) => setShareQuery(event.target.value)}
                />
              </label>
              <SelectMenu
                value={shareFilter}
                options={[
                  { value: "all", label: "全部链接", count: shareCounts.all },
                  { value: "active", label: "有效", count: shareCounts.active },
                  {
                    value: "expired",
                    label: "已过期",
                    count: shareCounts.expired,
                  },
                ]}
                onChange={(value) => setShareFilter(value as ShareFilter)}
                ariaLabel="筛选分享链接"
                className="share-filter-menu"
              />
            </div>
            {filteredShares.length > 0 && (
              <div className="share-batch-toolbar">
                <label className="checkbox-hit">
                  <input
                    type="checkbox"
                    aria-label="全选当前分享链接"
                    checked={filteredShares.every((item) =>
                      selectedShares.includes(item.token),
                    )}
                    onChange={toggleAllShares}
                  />
                </label>
                <span>
                  {selectedShares.length
                    ? `已选 ${selectedShares.length} 个`
                    : "选择分享链接"}
                </span>
                {selectedShares.length > 0 && (
                  <>
                    <button
                      className="danger-ghost"
                      onClick={() => setShareDeleteConfirm(true)}
                    >
                      <Icon name="trash" size={15} />
                      批量删除
                    </button>
                    <button
                      className="danger-ghost"
                      onClick={() => void disableSelectedShareAliases}
                    >
                      <Icon name="archive" size={15} />
                      批量停用
                    </button>
                    <button
                      className="text-button"
                      onClick={() => setSelectedShares([])}
                    >
                      清除选择
                    </button>
                  </>
                )}
              </div>
            )}
            {shares.length === 0 ? (
              <div className="empty-state small-empty">
                <h3>还没有分享链接。</h3>
                <p>需要共享时，可以为活跃别名创建只读链接。</p>
              </div>
            ) : filteredShares.length === 0 ? (
              <div className="empty-state small-empty">
                <h3>没有匹配的分享链接。</h3>
                <p>请尝试其他邮箱、标签或状态筛选。</p>
              </div>
            ) : (
              <div className="share-list">
                <ShareRows
                  groups={groupedShares}
                  selectedShares={selectedShares}
                  onToggle={toggleShareSelection}
                  onCopy={copyShare}
                  onEdit={openShareEditor}
                  onDisable={(email) => {
                    const item = aliases.find(
                      (candidate) => candidate.email === email,
                    );
                    if (item) void toggleAlias(item);
                  }}
                  onDelete={(item) => {
                    void (async () => {
                      try {
                        await api.deleteShare(item.token);
                        setShares((current) =>
                          current.filter((share) => share.token !== item.token),
                        );
                        setNotice("分享链接已删除。");
                      } catch (e) {
                        setNotice(`删除分享链接失败：${errorText(e)}`);
                      }
                    })();
                  }}
                />
              </div>
            )}
          </div>
        )}
      </section>
      <SidePanel
        open={addOpen}
        title="创建别名"
        onClose={() => setAddOpen(false)}
      >
        <form className="drawer-form" onSubmit={createAlias}>
          <p className="form-intro">
            在 <span className="mono">{account?.name}</span>{" "}
            下创建新的隐藏邮箱地址。名称同步到 iCloud，分组只用于本项目整理。
          </p>
          <label className="field">
            <span>名称 / 标签</span>
            <input
              value={label}
              onChange={(event) => setLabel(event.target.value)}
              placeholder="例如：供应商注册"
              autoFocus
            />
            <small>创建后会同时显示名称和 iCloud 地址。</small>
          </label>
          <label className="field">
            <span>所属分组</span>
            <SelectMenu
              value={newAliasGroupId}
              options={[
                { value: "", label: "未分组" },
                ...groups.map((group) => ({
                  value: group.id,
                  label: group.name,
                })),
              ]}
              onChange={setNewAliasGroupId}
              ariaLabel="选择新邮箱分组"
              className="field-select-menu"
              searchable
              searchPlaceholder="搜索分组"
            />
          </label>
          <div className="drawer-actions">
            <button
              type="button"
              className="button secondary"
              onClick={() => setAddOpen(false)}
            >
              取消
            </button>
            <button className="button primary" type="submit" disabled={busy}>
              {busy ? "创建中…" : "创建别名"}
              <Icon name="arrow" size={16} />
            </button>
          </div>
        </form>
      </SidePanel>
      <SidePanel
        open={batchOpen}
        title="批量创建别名"
        onClose={() => !busy && setBatchOpen(false)}
      >
        <form className="drawer-form" onSubmit={createAliasBatch}>
          <p className="form-intro">
            按统一规则创建邮箱，并一次性设置名称、分组和备注。单次最多 50 个。
          </p>
          <div className="batch-create-grid">
            <label className="field">
              <span>创建数量</span>
              <input type="number" min={1} max={50} value={batchCount} onChange={(event) => setBatchCount(Number(event.target.value))} required />
            </label>
            <label className="field">
              <span>名称前缀</span>
              <input value={batchPrefix} onChange={(event) => setBatchPrefix(event.target.value)} placeholder="例如：资格号" maxLength={180} required autoFocus />
            </label>
          </div>
          <div className="field">
            <span>命名规则</span>
            <SelectMenu
              value={batchNamingRule}
              options={[
                { value: "sequence", label: "递增编号" },
                { value: "same", label: "使用相同名称" },
              ]}
              onChange={(value) => setBatchNamingRule(value as "sequence" | "same")}
              ariaLabel="选择批量命名规则"
              className="field-select-menu"
            />
          </div>
          {batchNamingRule === "sequence" && (
            <div className="batch-create-grid batch-number-grid">
              <label className="field">
                <span>分隔符</span>
                <SelectMenu
                  value={batchSeparator}
                  options={[
                    { value: "-", label: "短横线 -" },
                    { value: "_", label: "下划线 _" },
                    { value: " ", label: "空格" },
                    { value: "", label: "不使用" },
                  ]}
                  onChange={setBatchSeparator}
                  ariaLabel="选择名称分隔符"
                  className="field-select-menu"
                />
              </label>
              <label className="field">
                <span>起始编号</span>
                <input type="number" min={1} max={99999999} value={batchStartNumber} onChange={(event) => setBatchStartNumber(Number(event.target.value))} required />
              </label>
              <label className="field">
                <span>编号位数</span>
                <input type="number" min={1} max={8} value={batchPadding} onChange={(event) => setBatchPadding(Number(event.target.value))} required />
              </label>
            </div>
          )}
          <div className="batch-name-preview">
            <span>名称预览</span>
            <strong>{[0, 1, 2].slice(0, Math.min(3, batchCount)).map(batchLabelPreview).join("、")}</strong>
          </div>
          <div className="field">
            <span>所属分组</span>
            <SelectMenu
              value={batchGroupId}
              options={[{ value: "", label: "未分组" }, ...groups.map((group) => ({ value: group.id, label: group.name }))]}
              onChange={setBatchGroupId}
              ariaLabel="选择批量创建邮箱的分组"
              className="field-select-menu"
              searchable
              searchPlaceholder="搜索分组"
            />
          </div>
          <label className="field">
            <span>统一备注 <small>可选</small></span>
            <textarea rows={4} value={batchNote} onChange={(event) => setBatchNote(event.target.value)} placeholder="例如：用于注册服务、客户联系或测试环境" maxLength={2000} />
          </label>
          <div className="drawer-actions">
            <button type="button" className="button secondary" onClick={() => setBatchOpen(false)} disabled={busy}>取消</button>
            <button type="submit" className="button primary" disabled={busy || !batchPrefix.trim()}>{busy ? "正在创建…" : `创建 ${batchCount} 个邮箱`}</button>
          </div>
        </form>
      </SidePanel>
      <SidePanel
        open={mailConfigOpen}
        title="邮件读取配置"
        onClose={() => setMailConfigOpen(false)}
      >
        <div className="mail-config-panel">
          <div className="mail-config-summary">
            <span className="eyebrow">当前默认方式</span>
            <strong>{mailMethodLabel(account?.mail_read_method || (mailMethod === "auto" ? "web_api" : mailMethod))}</strong>
          </div>
          <div className="mail-config-tabs" role="tablist" aria-label="邮件配置类型">
            <button
              type="button"
              className={mailConfigTab === "imap" ? "active" : ""}
              onClick={() => setMailConfigTab("imap")}
              role="tab"
              aria-selected={mailConfigTab === "imap"}
            >
              iCloud IMAP
            </button>
            <button
              type="button"
              className={mailConfigTab === "forward_imap" ? "active" : ""}
              onClick={() => setMailConfigTab("forward_imap")}
              role="tab"
              aria-selected={mailConfigTab === "forward_imap"}
            >
              转发邮箱 IMAP
            </button>
          </div>
          <div className="mail-config-method">
            <span className="eyebrow">默认读取方式</span>
            <div className="mail-config-method-row">
              <strong>{mailMethodLabel(mailConfigTab)}</strong>
              {mailMethod !== mailConfigTab && (
                <button
                  type="button"
                  className="button small primary mail-config-save"
                  onClick={() => {
                    setMailMethod(mailConfigTab);
                    void api.setMailReadMethod(id, mailConfigTab).then(() => {
                      setAccount((current) => current ? { ...current, mail_read_method: mailConfigTab } : current);
                      setMailMethodNotice("读取方式已保存到服务器。");
                    }).catch((error) => setMailMethodNotice((error as Error).message));
                  }}
                  disabled={false}
                >
                  保存
                </button>
              )}
            </div>
          </div>
          {mailConfigTab === "imap" ? (
            <AppPasswordForm accountId={id} account={account} onDone={refreshAfterPassword} />
          ) : (
            <ForwardIMAPForm
              accountId={id}
              account={account}
              onDone={(messageText) => {
                setNotice(messageText);
                void load();
              }}
            />
          )}
        </div>
      </SidePanel>
      <SidePanel
        open={shareOpen}
        title="创建分享链接"
        onClose={() => setShareOpen(false)}
      >
        <form className="drawer-form" onSubmit={createShare}>
          <p className="form-intro">
            分享链接只读访问收件箱。请只发送给可信的人。
          </p>
          <SelectMenu
            value={shareAlias}
            options={activeAliases.map((item) => ({
              value: item.email,
              label: item.email,
            }))}
            onChange={setShareAlias}
            ariaLabel="选择要分享的活跃别名"
            className="field-select-menu"
            searchable
            searchPlaceholder="搜索邮箱地址"
          />
          <label className="field">
            <span>
              链接备注 <small>可选</small>
            </span>
            <input
              value={shareLabel}
              onChange={(event) => setShareLabel(event.target.value)}
              placeholder="写下分享对象或使用场景"
            />
          </label>
          <div className="field">
            <span>有效期</span>
            <SelectMenu
              value={
                isShareDurationPreset(shareDuration) ? shareDuration : "custom"
              }
              options={[
                { value: "0", label: "永久有效" },
                { value: "120", label: "2 小时" },
                { value: "10080", label: "7 天" },
                { value: "20160", label: "14 天" },
                { value: "custom", label: "自定义" },
              ]}
              onChange={(value) =>
                setShareDuration(value === "custom" ? "" : value)
              }
              ariaLabel="选择分享链接有效期"
              className="share-expiry-menu"
            />
          </div>
          {!isShareDurationPreset(shareDuration) && (
            <>
              <label className="field">
                <span>自定义有效期</span>
                <input
                  type="text"
                  inputMode="text"
                  value={shareDuration}
                  onChange={(event) => setShareDuration(event.target.value)}
                  placeholder="例如：1h、1h30m、14d"
                  required
                />
              </label>
              <small className="share-duration-help">
                支持 m、h、d、w，可组合到分钟。
              </small>
            </>
          )}
          <div className="drawer-actions">
            <button
              type="button"
              className="button secondary"
              onClick={() => setShareOpen(false)}
            >
              取消
            </button>
            <button
              className="button primary"
              type="submit"
              disabled={shareCreating || !shareAlias}
            >
              {shareCreating ? "创建中…" : "创建链接"}
              <Icon name="link" size={16} />
            </button>
          </div>
        </form>
      </SidePanel>
      <SidePanel
        open={organizerOpen}
        title="管理本地分组"
        onClose={() => setOrganizerOpen(false)}
      >
        <div className="group-manager">
          <p className="group-manager-copy">
            分组只保存在本项目中，不会同步到 iCloud，也不会改变邮箱状态。
          </p>
          <form className="group-create-form" onSubmit={createGroup}>
            <label className="field">
              <span>新分组名称</span>
              <input
                value={groupName}
                onChange={(event) => setGroupName(event.target.value)}
                placeholder="例如：账单、测试、客户"
                autoFocus
              />
            </label>
            <button className="button primary" type="submit" disabled={busy}>
              创建分组
            </button>
          </form>
          <div className="group-list-heading">
            <span>分组</span>
            <span>别名数</span>
          </div>
          {groupOrderDirty && (
            <div className="group-order-bar" role="status">
              <span>顺序已调整，保存后对当前账号生效。</span>
              <button className="button primary small" type="button" disabled={busy} onClick={() => void saveGroupOrder()}>
                {busy ? "保存中…" : "保存排序"}
              </button>
            </div>
          )}
          <div
            ref={groupListRef}
            className={`group-list ${draggedGroupId ? "group-list-reordering" : ""}`}
            onPointerMove={(event) => moveGroupDrag(event.pointerId, event.clientX, event.clientY)}
            onPointerUp={finishGroupDrag}
            onPointerCancel={finishGroupDrag}
            onLostPointerCapture={finishGroupDrag}
          >
            {groups.length === 0 ? (
              <p className="group-empty">还没有自定义分组。</p>
            ) : (
              groups.map((group) => (
                <div
                  className={`group-row ${draggedGroupId === group.id ? "group-row-dragging" : ""} ${dragOverGroupId === group.id && draggedGroupId !== group.id ? "group-row-drop-target" : ""}`}
                  key={group.id}
                  data-group-id={group.id}
                  ref={(row) => {
                    if (row) groupRowRefs.current.set(group.id, row);
                    else groupRowRefs.current.delete(group.id);
                  }}
                >
                  {renameGroupId === group.id ? (
                    <form
                      className="group-rename"
                      onSubmit={(event) => {
                        event.preventDefault();
                        void renameGroup(group);
                      }}
                    >
                      <input
                        value={renameValue}
                        onChange={(event) => setRenameValue(event.target.value)}
                        aria-label={`重命名 ${group.name}`}
                        autoFocus
                      />
                      <button className="row-action" aria-label="保存分组名称">
                        <Icon name="check" size={16} />
                      </button>
                      <button
                        className="row-action"
                        type="button"
                        aria-label="取消重命名"
                        onClick={() => setRenameGroupId(null)}
                      >
                        <Icon name="close" size={16} />
                      </button>
                    </form>
                  ) : (
                    <>
                      <div className="group-row-main">
                        <button
                          type="button"
                          className="group-drag-handle"
                          aria-label={`拖动排序 ${group.name}`}
                          title="拖动排序"
                          onPointerDown={(event) => {
                            if (renameGroupId === group.id) return;
                            if (!event.isPrimary || (event.pointerType === "mouse" && event.button !== 0)) return;
                            event.preventDefault();
                            groupDrag.current = {
                              pointerId: event.pointerId,
                              sourceId: group.id,
                              startY: event.clientY,
                              pointerY: event.clientY,
                              grabOffsetY:
                                event.clientY -
                                (groupRowRefs.current.get(group.id)?.getBoundingClientRect().top || event.clientY),
                              translateY: 0,
                            };
                            setDraggedGroupId(group.id);
                            setDragOverGroupId(group.id);
                            groupListRef.current?.setPointerCapture(event.pointerId);
                          }}
                        >
                          ⠿
                        </button>
                        <span className="group-row-copy">
                          <strong>{group.name}</strong>
                          <small>{allGroupCounts[group.id] || 0} 个别名</small>
                        </span>
                      </div>
                      <div className="group-row-actions">
                        <button
                          className="row-action"
                          aria-label={`重命名 ${group.name}`}
                          onClick={() => {
                            setRenameGroupId(group.id);
                            setRenameValue(group.name);
                          }}
                        >
                          <Icon name="settings" size={16} />
                        </button>
                        <button
                          className="row-action"
                          aria-label={`删除 ${group.name}`}
                          onClick={() => setGroupDeleteTarget(group)}
                        >
                          <Icon name="trash" size={16} />
                        </button>
                      </div>
                    </>
                  )}
                </div>
              ))
            )}
          </div>
        </div>
      </SidePanel>
      <SidePanel
        open={editorAlias !== null}
        title="编辑别名"
        onClose={() => setEditorAlias(null)}
      >
        <form className="drawer-form" onSubmit={saveAliasMeta}>
          <p className="form-intro">
            名称会同步到 iCloud；分组和备注只保存在本项目中。
          </p>
          <p className="editor-address mono">{editorAlias?.email}</p>
          <p className="editor-created-at">
            {parseDateValue(editorAlias?.createdAt)
              ? `创建于 ${dateText(editorAlias?.createdAt)}`
              : "创建时间未知"}
          </p>
          <label className="field">
            <span>别名名称</span>
            <input
              value={editorLabel}
              onChange={(event) => setEditorLabel(event.target.value)}
              placeholder="例如：注册、账单、客户"
              maxLength={200}
              required
            />
            <small>这是创建时填写的名称，可以随时修改。</small>
          </label>
          <SelectMenu
            value={editorGroupId}
            options={[
              { value: "", label: "未分组" },
              ...groups.map((group) => ({
                value: group.id,
                label: group.name,
              })),
            ]}
            onChange={setEditorGroupId}
            ariaLabel="选择本地分组"
            className="field-select-menu"
            searchable
            searchPlaceholder="搜索分组"
          />
          <label className="field">
            <span>备注</span>
            <textarea
              rows={5}
              value={editorNote}
              onChange={(event) => setEditorNote(event.target.value)}
              placeholder="写下这个地址的用途或维护提示"
            />
          </label>
          <div className="drawer-actions">
            <button
              className="button secondary"
              type="button"
              onClick={() => setEditorAlias(null)}
            >
              取消
            </button>
            <button className="button primary" type="submit" disabled={busy}>
              {busy ? "保存中…" : "保存本地信息"}
              <Icon name="check" size={16} />
            </button>
          </div>
        </form>
      </SidePanel>
      <Dialog
        open={shareResult !== null}
        title="分享链接已创建"
        onClose={() => setShareResult(null)}
      >
        <div className="dialog-body">
          <p className="dialog-lead">请复制并发送给需要只读访问的人。</p>
          <p className="share-result-url mono">{shareResult}</p>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setShareResult(null)}
            >
              关闭
            </button>
            <button
              className="button primary"
              onClick={() => {
                if (shareResult) {
                  navigator.clipboard.writeText(shareResult);
                  setNotice("分享链接已复制。");
                }
              }}
            >
              <Icon name="copy" size={15} />
              复制链接
            </button>
          </div>
        </div>
      </Dialog>{" "}
      <Dialog
        open={shareEditTarget !== null}
        title="修改分享备注"
        onClose={() => !busy && setShareEditTarget(null)}
      >
        <form className="dialog-body" onSubmit={saveShareLabel}>
          <p className="dialog-lead">
            备注只用于区分分享链接，不会改变链接地址、有效期或邮箱备注。
          </p>
          <p className="editor-address mono">{shareEditTarget?.alias}</p>
          <label className="field">
            <span>链接备注</span>
            <input
              value={shareEditLabel}
              onChange={(event) => setShareEditLabel(event.target.value)}
              placeholder="写下分享对象或使用场景"
              maxLength={200}
              autoFocus
            />
            <small>留空后将显示为“未命名链接”。</small>
          </label>
          <div className="dialog-actions">
            <button type="button" className="button secondary" disabled={busy} onClick={() => setShareEditTarget(null)}>取消</button>
            <button type="submit" className="button primary" disabled={busy}>{busy ? "保存中…" : "保存备注"}</button>
          </div>
        </form>
      </Dialog>
      <Dialog
        open={createdAlias !== ""}
        title="邮箱已创建"
        onClose={() => setCreatedAlias("")}
      >
        <div className="dialog-body">
          <p className="dialog-lead">新邮箱地址如下，可以直接复制使用。</p>
          <p className="share-result-url mono">{createdAlias}</p>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setCreatedAlias("")}
            >
              关闭
            </button>
            <button
              className="button primary"
              onClick={() => {
                navigator.clipboard.writeText(createdAlias);
                setNotice("邮箱地址已复制。");
              }}
            >
              <Icon name="copy" size={15} />
              复制邮箱
            </button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={batchResult !== null}
        title="批量创建结果"
        onClose={() => setBatchResult(null)}
        wide
      >
        <div className="dialog-body">
          <p className="dialog-lead">
            成功 {batchResult?.succeeded || 0} 个，失败 {batchResult?.failed || 0} 个
            {batchResult?.metadata_failed ? `，其中 ${batchResult.metadata_failed} 个未保存分组或备注` : ""}。
          </p>
          <div className="batch-result-list">
            {batchResult?.results.map((item) => (
              <div className={`batch-result-row ${item.success ? "success" : "failed"}`} key={item.index}>
                <span>{String(item.index).padStart(2, "0")}</span>
                <div>
                  <strong>{item.label || "未命名"}</strong>
                  <small className="mono">{item.email || item.error || "未创建"}</small>
                  {item.metadata_error && <small className="batch-meta-error">{item.metadata_error}</small>}
                </div>
                {item.success && item.email && (
                  <button className="icon-button" aria-label={`复制 ${item.email}`} title="复制邮箱" onClick={() => { navigator.clipboard.writeText(item.email || ""); setNotice("邮箱地址已复制。"); }}>
                    <Icon name="copy" size={16} />
                  </button>
                )}
              </div>
            ))}
          </div>
          <div className="dialog-actions">
            <button className="button secondary" onClick={() => setBatchResult(null)}>关闭</button>
            <button className="button primary" disabled={!batchResult?.succeeded} onClick={() => { const emails = batchResult?.results.filter((item) => item.success && item.email).map((item) => item.email).join("\n") || ""; navigator.clipboard.writeText(emails); setNotice(`已复制 ${batchResult?.succeeded || 0} 个邮箱地址。`); }}>
              <Icon name="copy" size={15} />复制全部邮箱
            </button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={shareDeleteConfirm}
        title="删除所选分享链接？"
        onClose={() => setShareDeleteConfirm(false)}
      >
        <div className="dialog-body">
          <div className="dialog-warning destructive">
            <Icon name="trash" size={20} />
            <div>
              <strong>此操作无法撤销。</strong>
              <p>
                将永久删除当前筛选中已选择的 {selectedShares.length}{" "}
                个分享链接。
              </p>
            </div>
          </div>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setShareDeleteConfirm(false)}
            >
              取消
            </button>
            <button
              className="button danger"
              disabled={busy}
              onClick={deleteSelectedShares}
            >
              {busy ? "删除中…" : "确认删除"}
            </button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={activeDeleteConfirm}
        title="停用所选邮箱？"
        onClose={() => setActiveDeleteConfirm(false)}
      >
        <div className="dialog-body">
          <div className="dialog-warning">
            <Icon name="archive" size={20} />
            <div>
              <strong>所选邮箱会移入停用邮箱区。</strong>
              <p>将停用 {selectedActive.length} 个隐藏邮箱，之后仍可恢复。</p>
            </div>
          </div>
          <div className="dialog-actions">
            <button className="button secondary" onClick={() => setActiveDeleteConfirm(false)}>取消</button>
            <button className="button primary" disabled={busy} onClick={disableSelectedActive}>{busy ? "停用中…" : "确认停用"}</button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={messageDeleteConfirm}
        title="删除这封邮件？"
        onClose={() => setMessageDeleteConfirm(false)}
      >
        <div className="dialog-body">
          <div className="dialog-warning destructive">
            <Icon name="trash" size={20} />
            <div>
              <strong>此操作无法撤销。</strong>
              <p>邮件会从当前 IMAP 收件箱中永久删除。</p>
            </div>
          </div>
          <div className="dialog-actions">
            <button className="button secondary" onClick={() => setMessageDeleteConfirm(false)}>取消</button>
            <button className="button danger" disabled={messageDeleting} onClick={deleteMessage}>{messageDeleting ? "删除中…" : "确认删除"}</button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={messageBatchDeleteConfirm}
        title="批量删除邮件？"
        onClose={() => setMessageBatchDeleteConfirm(false)}
      >
        <div className="dialog-body">
          <div className="dialog-warning destructive">
            <Icon name="trash" size={20} />
            <div>
              <strong>此操作无法撤销。</strong>
              <p>将从当前 IMAP 收件箱中永久删除所选的 {selectedMessages.length} 封邮件。</p>
            </div>
          </div>
          <div className="dialog-actions">
            <button className="button secondary" onClick={() => setMessageBatchDeleteConfirm(false)}>取消</button>
              <button className="button danger" disabled={messageDeleting} onClick={deleteSelectedMessages}>{messageDeleting ? "删除中…" : "确认删除"}</button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={deleteTarget !== null}
        title="删除这个别名？"
        onClose={() => setDeleteTarget(null)}
      >
        <div className="dialog-body">
          <div className="dialog-warning destructive">
            <Icon name="trash" size={20} />
            <div>
              <strong>此操作无法撤销。</strong>
              <p>
                <span className="mono">{deleteTarget?.email}</span>{" "}
                及其本地记录会从此账号中删除。
              </p>
            </div>
          </div>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setDeleteTarget(null)}
            >
              取消
            </button>
            <button
              className="button danger"
              disabled={busy}
              onClick={deleteAlias}
            >
              {busy ? "删除中…" : "确认删除"}
            </button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={disabledDeleteConfirm}
        title="删除所选停用邮箱？"
        onClose={() => setDisabledDeleteConfirm(false)}
      >
        <div className="dialog-body">
          <div className="dialog-warning destructive">
            <Icon name="trash" size={20} />
            <div>
              <strong>此操作无法撤销。</strong>
              <p>
                将永久删除所选的 {selectedDisabled.length}{" "}
                个停用别名及其本地记录。
              </p>
            </div>
          </div>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setDisabledDeleteConfirm(false)}
            >
              取消
            </button>
            <button
              className="button danger"
              disabled={busy}
              onClick={deleteSelectedDisabled}
            >
              {busy ? "删除中…" : "确认删除"}
            </button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={groupDeleteTarget !== null}
        title="删除这个本地分组？"
        onClose={() => setGroupDeleteTarget(null)}
      >
        <div className="dialog-body">
          <div className="dialog-warning">
            <Icon name="archive" size={20} />
            <div>
              <strong>别名不会被删除。</strong>
              <p>这里只会移除本地分组；原有别名仍保留，只会变为未分组。</p>
            </div>
          </div>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setGroupDeleteTarget(null)}
            >
              取消
            </button>
            <button
              className="button danger"
              disabled={busy}
              onClick={deleteGroup}
            >
              {busy ? "删除中…" : "确认删除分组"}
            </button>
          </div>
        </div>
      </Dialog>
    </PageLayout>
  );
}
function ForwardIMAPForm({
  accountId,
  account,
  onDone,
}: {
  accountId: string;
  account: Account | null;
  onDone: (message: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(true);
  const saved = account?.forward_imap;
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    void api.getForwardIMAP(accountId).then((config) => {
      if (!cancelled) setPassword(config.password || "");
    }).catch((caught) => {
      if (!cancelled) setError(errorText(caught));
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [accountId]);
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    const values = Object.fromEntries(
      new FormData(event.currentTarget).entries(),
    ) as Record<string, string>;
    try {
      await api.setForwardIMAP(accountId, {
        host: values.host.trim(),
        port: Number(values.port || 993),
        email: values.email.trim(),
        password,
        mailboxes: values.mailboxes
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
      });
      onDone("转发邮箱 IMAP 验证通过，之后会优先从这里读取邮件。");
    } catch (caught) {
      setError(errorText(caught));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="drawer-form" onSubmit={submit}>
      <p className="form-intro">
        填写隐藏邮箱最终转发到的真实邮箱。连接全程使用 TLS；Gmail
        使用应用专用密码，QQ 邮箱使用 IMAP 授权码。
      </p>
      <label className="field">
        <span>IMAP 服务器</span>
        <input
          name="host"
          defaultValue={saved?.host || ""}
          placeholder="imap.gmail.com"
          autoComplete="off"
          required
        />
        <small>
          Gmail: imap.gmail.com · QQ: imap.qq.com · Outlook:
          outlook.office365.com
        </small>
      </label>
      <label className="field">
        <span>端口</span>
        <input
          name="port"
          type="number"
          min="1"
          max="65535"
          defaultValue={saved?.port || 993}
          required
        />
      </label>
      <label className="field">
        <span>转发邮箱账号</span>
        <input
          name="email"
          type="email"
          defaultValue={saved?.email || ""}
          placeholder="name@example.com"
          autoComplete="username"
          required
        />
      </label>
      <label className="field">
        <span>应用专用密码或授权码</span>
        <PasswordInput
          name="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          placeholder={
            account?.has_forward_imap ? "已保存，可直接使用或修改" : "请输入授权码"
          }
          autoComplete="current-password"
          required
        />
      </label>
      <label className="field">
        <span>扫描文件夹</span>
        <input
          name="mailboxes"
          defaultValue={(saved?.mailboxes || ["INBOX"]).join(", ")}
          placeholder="INBOX, [Gmail]/垃圾邮件"
        />
        <small>
          多个文件夹用英文逗号分隔。Gmail 中文账号可尝试「INBOX, [Gmail]/垃圾邮件」；如果仍失败，请以该账号 IMAP LIST 返回的实际名称为准，不要额外填写 spam。
        </small>
      </label>
      {error && (
        <div className="form-error" role="alert">
          {error}
        </div>
      )}
      <div className="drawer-actions">
        <button className="button primary" type="submit" disabled={busy || loading || !password}>
          {loading ? "正在读取配置…" : busy ? "正在验证…" : "验证并保存"}
          <Icon name="check" size={16} />
        </button>
      </div>
    </form>
  );
}
function AppPasswordForm({
  accountId,
  account,
  onDone,
}: {
  accountId: string;
  account: Account | null;
  onDone: (message: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    void api.getAppPassword(accountId).then((saved) => {
      if (cancelled) return;
      setEmail(saved.icloud_email || "");
      setPassword(saved.app_password || "");
    }).catch((caught) => {
      if (!cancelled) setError(errorText(caught));
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [accountId]);
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.setAppPassword(accountId, email, password);
      onDone("App 专用密码验证通过，已启用 IMAP 阅读。");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="drawer-form" onSubmit={submit}>
      <p className="form-intro">
        iCloud 邮件只需要实际的 iCloud 邮箱和 Apple 生成的 App 专用密码。IMAP
        服务器、端口和加密方式已内置，无需重复填写。
      </p>
      <label className="field">
        <span>iCloud 邮箱</span>
        <input
          name="email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          placeholder="name@icloud.com"
          pattern=".+@(icloud\.com|me\.com|mac\.com)"
          title="请输入 @icloud.com、@me.com 或 @mac.com 邮箱"
          autoComplete="username"
          required
        />
      </label>
      <label className="field">
        <span>App 专用密码</span>
        <PasswordInput
          name="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          placeholder={
            account?.has_app_password
              ? "已保存，可直接使用或修改"
              : "xxxx-xxxx-xxxx-xxxx"
          }
          autoComplete="current-password"
          required
        />
      </label>
      {error && (
        <div className="form-error" role="alert">
          {error}
        </div>
      )}
      <div className="drawer-actions">
        <button className="button primary" type="submit" disabled={busy || loading || !email || !password}>
          {loading ? "正在读取配置…" : busy ? "验证中…" : account?.has_app_password ? "保存并验证" : "验证并启用邮件"}
          <Icon name="check" size={16} />
        </button>
      </div>
    </form>
  );
}
