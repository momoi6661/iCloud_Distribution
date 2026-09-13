import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, type Account, type BatchAccountResult } from "../api/client";
import Icon from "../components/Icon";
import PageLayout from "../components/PageLayout";
import Pagination from "../components/Pagination";
import { Dialog, SidePanel } from "../components/Overlay";
import SelectMenu from "../components/SelectMenu";

function batchNotice(
  result: BatchAccountResult,
  action: "deactivate" | "delete",
) {
  const requested = result.requested ?? 0;
  const changed =
    result.changed ?? (action === "delete" ? (result.deleted ?? 0) : 0);
  const alreadyDisabled = result.already_disabled ?? 0;
  const notFound = result.not_found ?? 0;
  const failed = result.failed ?? 0;
  if (action === "delete")
    return `已删除 ${result.deleted ?? changed} 个账号（请求 ${requested} 个）${notFound ? `，${notFound} 个未找到` : ""}${failed ? `，${failed} 个失败` : ""}。`;
  return `已停用 ${changed} 个账号（请求 ${requested} 个）${alreadyDisabled ? `，${alreadyDisabled} 个原已停用` : ""}${notFound ? `，${notFound} 个未找到` : ""}${failed ? `，${failed} 个失败` : ""}。`;
}

const dateText = (value?: string) =>
  value ? new Date(value).toLocaleDateString("zh-CN") : "—";
const PAGE_SIZE = 20;

export default function AccountsPage({ onLogout }: { onLogout: () => void }) {
  const navigate = useNavigate();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [loginTarget, setLoginTarget] = useState<Account | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [page, setPage] = useState(1);
  const [countsLoading, setCountsLoading] = useState<string[]>([]);

  const refresh = async () => {
    setLoading(true);
    try {
      const listed = (await api.listAccounts()).filter(
        (account) => account.status !== "disabled",
      );
      setAccounts(listed);
      setCountsLoading(listed.map((account) => account.id));
      void Promise.all(
        listed.map(async (account) => {
          try {
            const result = await api.listAliases(account.id);
            const aliases = result.aliases || [];
            setAccounts((current) =>
              current.map((item) =>
                item.id === account.id
                  ? {
                      ...item,
                      alias_total: aliases.length,
                      alias_active: aliases.filter((alias) => alias.active)
                        .length,
                    }
                  : item,
              ),
            );
          } catch {
            // Keep the saved summary when a live alias refresh is unavailable.
          } finally {
            setCountsLoading((current) =>
              current.filter((id) => id !== account.id),
            );
          }
        }),
      );
    } catch (error) {
      setNotice((error as Error).message);
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void refresh();
  }, []);
  useEffect(() => {
    if (!notice) return undefined;
    const timer = window.setTimeout(() => setNotice(""), 4200);
    return () => window.clearTimeout(timer);
  }, [notice]);

  const visibleAccounts = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return accounts;
    return accounts.filter((account) =>
      [
        account.name,
        account.icloud_email,
        account.real_email,
        account.host,
      ].some((value) => value?.toLowerCase().includes(needle)),
    );
  }, [accounts, query]);
  const totalPages = Math.max(1, Math.ceil(visibleAccounts.length / PAGE_SIZE));
  const pagedAccounts = useMemo(
    () => visibleAccounts.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [page, visibleAccounts],
  );
  useEffect(() => {
    setPage(1);
  }, [query]);
  useEffect(() => {
    setPage((current) => Math.min(current, totalPages));
  }, [totalPages]);
  const allSelected =
    pagedAccounts.length > 0 &&
    pagedAccounts.every((account) => selected.includes(account.id));
  const toggle = (id: string) =>
    setSelected((current) =>
      current.includes(id)
        ? current.filter((item) => item !== id)
        : [...current, id],
    );
  const toggleAll = () =>
    setSelected(
      allSelected
        ? selected.filter(
            (id) => !pagedAccounts.some((account) => account.id === id),
          )
        : [
            ...new Set([
              ...selected,
              ...pagedAccounts.map((account) => account.id),
            ]),
          ],
    );

  const deactivateSelected = async () => {
    if (!selected.length) return;
    setBusy(true);
    try {
      const result = await api.batchDisableAccounts(selected);
      setNotice(batchNotice(result, "deactivate"));
      setSelected([]);
      await refresh();
    } catch (error) {
      setNotice((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const deleteSelected = async () => {
    if (!selected.length) return;
    setBusy(true);
    try {
      const result = await api.batchDeleteAccounts(selected);
      setNotice(batchNotice(result, "delete"));
      setDeleteConfirm(false);
      setSelected([]);
      await refresh();
    } catch (error) {
      setNotice((error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <PageLayout title="活跃账号" eyebrow="运维总览" onLogout={onLogout}>
      {notice && (
        <div className="notice toast" role="status" aria-live="polite">
          <span>{notice}</span>
          <button className="text-button" onClick={() => setNotice("")}>
            关闭
          </button>
        </div>
      )}
      <section className="metrics-grid" aria-label="账号概览">
        <div className="metric-card">
          <span>活跃账号</span>
          <strong>{accounts.length}</strong>
          <small>当前可管理</small>
        </div>
        <div className="metric-card">
          <span>活跃别名</span>
          <strong>
            {accounts.reduce(
              (sum, account) => sum + (account.alias_active || 0),
              0,
            )}
          </strong>
          <small>正在转发</small>
        </div>
        <div className="metric-card">
          <span>待处理</span>
          <strong>
            {
              accounts.filter(
                (account) =>
                  account.status === "pending" || account.status === "error",
              ).length
            }
          </strong>
          <small>需要检查</small>
        </div>
        <div className="metric-card">
          <span>已选账号</span>
          <strong>{selected.length}</strong>
          <small>批量操作范围</small>
        </div>
      </section>
      <section className="panel account-panel" aria-labelledby="accounts-title">
        <div className="panel-header">
          <div>
            <span className="eyebrow">账号矩阵</span>
            <h2 id="accounts-title">
              账号列表{" "}
              <span className="count-badge">{visibleAccounts.length}</span>
            </h2>
          </div>
          <div className="inline-actions">
            <label className="search-field">
              <Icon name="search" size={16} />
              <input
                aria-label="搜索账号"
                placeholder="搜索名称、邮箱或主机"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            <button className="button primary" onClick={() => setAddOpen(true)}>
              添加账号
            </button>
          </div>
        </div>
        {selected.length > 0 && (
          <div className="batch-toolbar">
            <span>
              <strong>{selected.length}</strong> 个已选
            </span>
            <span className="toolbar-separator" />
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void deactivateSelected}
            >
              <Icon name="archive" size={16} />
              批量停用
            </button>
            <button
              className="danger-ghost"
              disabled={busy}
              onClick={() => setDeleteConfirm(true)}
            >
              <Icon name="trash" size={16} />
              批量删除
            </button>
            <button className="text-button" onClick={() => setSelected([])}>
              清除选择
            </button>
          </div>
        )}
        {loading ? (
          <div className="skeleton-list">
            {[1, 2, 3].map((item) => (
              <div className="skeleton-row" key={item}>
                <span />
                <span />
                <span />
                <span />
              </div>
            ))}
          </div>
        ) : visibleAccounts.length === 0 ? (
          <div className="empty-state">
            <div className="empty-glyph">
              <Icon name="grid" size={22} />
            </div>
            <h3>{query ? "没有匹配的账号。" : "还没有活跃账号。"}</h3>
            <p>
              {query
                ? "请尝试名称、邮箱或主机的其他关键词。"
                : "添加第一个账号后，可以在这里管理别名和收件箱。"}
            </p>
            {!query && (
              <button
                className="button primary"
                onClick={() => setAddOpen(true)}
              >
                添加账号
              </button>
            )}
          </div>
        ) : (
          <div className="table-wrap">
            <table className="account-table">
              <thead>
                <tr>
                  <th className="check-col">
                    <label className="checkbox-hit">
                      <input
                        type="checkbox"
                        aria-label="全选当前账号"
                        checked={allSelected}
                        onChange={toggleAll}
                      />
                    </label>
                  </th>
                  <th>账号</th>
                  <th>状态</th>
                  <th>别名</th>
                  <th>最近验证</th>
                  <th className="action-col">
                    <span className="sr-only">操作</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {pagedAccounts.map((account, index) => (
                  <tr
                    key={account.id}
                    className={`${selected.includes(account.id) ? "selected" : ""} status-row-${account.status}`}
                    onClick={() => navigate(`/accounts/${account.id}`)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        navigate(`/accounts/${account.id}`);
                      }
                    }}
                    tabIndex={0}
                    style={{ animationDelay: `${index * 35}ms` }}
                  >
                    <td
                      className="check-col"
                      onClick={(event) => event.stopPropagation()}
                    >
                      <label className="checkbox-hit">
                        <input
                          type="checkbox"
                          aria-label={`选择 ${account.name}`}
                          checked={selected.includes(account.id)}
                          onChange={() => toggle(account.id)}
                        />
                      </label>
                    </td>
                    <td>
                      <div className="account-identity">
                        <span
                          className={`account-sigil sigil-${account.status}`}
                        >
                          {account.name.slice(0, 1).toUpperCase()}
                        </span>
                        <span>
                          <strong>{account.name}</strong>
                          <small className="mono">
                            {account.real_email ||
                              account.icloud_email ||
                              account.id}
                          </small>
                        </span>
                      </div>
                    </td>
                    <td>
                      <span
                        className={`status status-${account.status === "active" ? "ready" : account.status === "error" ? "error" : "pending"}`}
                      >
                        {account.status === "active"
                          ? "正常"
                          : account.status === "error"
                            ? "异常"
                            : "待处理"}
                      </span>
                    </td>
                    <td>
                      <span className="alias-load">
                        {countsLoading.includes(account.id) ? (
                          <small>同步中…</small>
                        ) : (
                          <>
                            <strong>{account.alias_active || 0}</strong>
                            <small> / {account.alias_total || 0} 个活跃</small>
                          </>
                        )}
                      </span>
                    </td>
                    <td>
                      <span className="validation-time">
                        {dateText(account.last_validated)}
                      </span>
                    </td>
                    <td className="action-col">
                      <div className="row-actions">
                        <button
                          className="button small secondary"
                          onClick={(event) => {
                            event.stopPropagation();
                            setLoginTarget(account);
                          }}
                        >
                          自动授权
                        </button>
                        <button
                          className="button small secondary"
                          onClick={(event) => {
                            event.stopPropagation();
                            navigate(`/accounts/${account.id}`);
                          }}
                        >
                          查看详情
                          <Icon name="arrow" size={15} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {totalPages > 1 && (
          <Pagination
            page={page}
            totalPages={totalPages}
            onChange={setPage}
            label="账号分页"
          />
        )}
      </section>
      <SidePanel
        open={addOpen}
        title="添加账号"
        onClose={() => setAddOpen(false)}
      >
        <AddAccountForm
          busy={busy}
          onSubmit={async (values) => {
            setBusy(true);
            try {
              await api.addAccount(values);
              setAddOpen(false);
              setNotice("账号已添加，请在列表中点击「自动授权」完成登录。");
              await refresh();
            } catch (error) {
              setNotice((error as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        />
      </SidePanel>
      <SidePanel
        open={Boolean(loginTarget)}
        title={`自动授权${loginTarget ? ` · ${loginTarget.name}` : ""}`}
        onClose={() => setLoginTarget(null)}
      >
        {loginTarget && (
          <AutoLoginForm
            account={loginTarget}
            onDone={async () => {
              setLoginTarget(null);
              setNotice("授权成功，Cookie 已自动保存。");
              await refresh();
            }}
          />
        )}
      </SidePanel>
      <Dialog
        open={deleteConfirm}
        title="删除所选账号？"
        onClose={() => setDeleteConfirm(false)}
      >
        <div className="dialog-body">
          <div className="dialog-warning destructive">
            <Icon name="trash" size={20} />
            <div>
              <strong>此操作无法撤销。</strong>
              <p>
                将永久删除所选 {selected.length}{" "}
                个账号及其本地凭据、别名记录。请确认这是明确的清理操作。
              </p>
            </div>
          </div>
          <div className="dialog-actions">
            <button
              className="button secondary"
              onClick={() => setDeleteConfirm(false)}
            >
              取消
            </button>
            <button
              className="button danger"
              disabled={busy}
              onClick={() => void deleteSelected()}
            >
              {busy ? "删除中…" : "确认永久删除"}
            </button>
          </div>
        </div>
      </Dialog>
    </PageLayout>
  );
}

function AddAccountForm({
  busy,
  onSubmit,
}: {
  busy: boolean;
  onSubmit: (values: {
    name: string;
    email?: string;
    cookies?: string;
    host?: string;
    proxy?: string;
  }) => Promise<void>;
}) {
  const [host, setHost] = useState("icloud.com.cn");
  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = Object.fromEntries(
      new FormData(event.currentTarget).entries(),
    ) as Record<string, string>;
    await onSubmit({
      name: values.name.trim(),
      email: values.email.trim() || undefined,
      cookies: values.cookies.trim() || undefined,
      host,
      proxy: values.proxy.trim() || undefined,
    });
  };
  return (
    <form className="drawer-form" onSubmit={submit}>
      <p className="form-intro">
        先创建账号资料，再从列表进入「自动授权」完成 iCloud
        登录。登录密码不会保存。
      </p>
      <label className="field">
        <span>显示名称</span>
        <input name="name" placeholder="例如：团队收件箱" required autoFocus />
      </label>
      <label className="field">
        <span>iCloud 邮箱</span>
        <input
          name="email"
          type="email"
          placeholder="operator@icloud.com"
          autoComplete="username"
        />
      </label>
      <label className="field">
        <span>服务主机</span>
        <SelectMenu
          value={host}
          options={[
            { value: "icloud.com", label: "icloud.com" },
            { value: "icloud.com.cn", label: "icloud.com.cn" },
          ]}
          onChange={setHost}
          ariaLabel="选择服务主机"
          className="field-select-menu"
        />
      </label>
      <label className="field">
        <span>
          Cookies <small>可选，已有会话时使用</small>
        </span>
        <textarea name="cookies" rows={4} placeholder="粘贴现有登录 Cookies" />
      </label>
      <label className="field">
        <span>
          代理地址 <small>可选</small>
        </span>
        <input name="proxy" placeholder="http://127.0.0.1:7890" />
      </label>
      <div className="drawer-actions">
        <button className="button primary" type="submit" disabled={busy}>
          {busy ? "添加中…" : "添加账号"}
          <Icon name="arrow" size={16} />
        </button>
      </div>
    </form>
  );
}

function AutoLoginForm({
  account,
  onDone,
}: {
  account: Account;
  onDone: () => Promise<void>;
}) {
  const [step, setStep] = useState<0 | 1>(0);
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [code, setCode] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [method, setMethod] = useState<"device" | "sms">("device");
  const [phones, setPhones] = useState<
    { id: number; numberWithDialCode: string }[]
  >([]);
  const [phoneId, setPhoneId] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const submitPassword = async () => {
    if (!password) return setError("请输入 iCloud 登录密码。");
    setBusy(true);
    setError("");
    try {
      const result = await api.loginStart(account.id, password);
      if (result.status === "done") return onDone();
      setSessionId(result.session_id || "");
      setPhones(result.phones || []);
      setPhoneId(result.phones?.[0]?.id || 0);
      setMethod(result.method || "device");
      setStep(1);
      if (result.warning && !result.sms_sent) setError(result.warning);
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const switchSMS = async () => {
    setBusy(true);
    setError("");
    try {
      const nextPhones = phones.length
        ? phones
        : (await api.loginPhones(account.id, sessionId)).phones || [];
      if (!nextPhones.length)
        throw new Error("暂时无法获取受信任手机号，请使用设备推送。");
      setPhones(nextPhones);
      setPhoneId(nextPhones[0].id);
      setMethod("sms");
      await api.loginSMS(account.id, sessionId, nextPhones[0].id);
    } catch (error) {
      setMethod("device");
      setError(
        `短信通道暂时不可用：${(error as Error).message}。如果你已经收到短信，仍可先尝试输入验证码。`,
      );
    } finally {
      setBusy(false);
    }
  };
  const sendSMS = async (id: number) => {
    setPhoneId(id);
    setBusy(true);
    try {
      await api.loginSMS(account.id, sessionId, id);
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const submitOTP = async () => {
    if (!/^\d{6}$/.test(code)) return setError("请输入 6 位数字验证码。");
    setBusy(true);
    setError("");
    try {
      await api.loginOTP(account.id, sessionId, code, method, phoneId);
      await onDone();
    } catch (error) {
      setError((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="drawer-form auth-wizard">
      <div className="wizard-steps">
        <span className={step === 0 ? "active" : "done"}>1 登录密码</span>
        <span className={step === 1 ? "active" : ""}>2 双重验证</span>
      </div>
      {step === 0 ? (
        <>
          <p className="form-intro">
            输入 iCloud 登录密码，不是 App
            专用密码。密码仅用于本次授权，不会保存。
          </p>
          <label className="field">
            <span>iCloud 登录密码</span>
            <div className="password-field">
              <input
                type={showPassword ? "text" : "password"}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") void submitPassword();
                }}
                autoFocus
                placeholder="输入 Apple ID 登录密码"
              />
              <button
                type="button"
                className="icon-button"
                aria-label={showPassword ? "隐藏密码" : "显示密码"}
                onClick={() => setShowPassword((current) => !current)}
              >
                <Icon name={showPassword ? "eye-off" : "eye"} size={17} />
              </button>
            </div>
          </label>
          <div className="drawer-actions">
            <button
              className="button primary"
              type="button"
              onClick={() => void submitPassword()}
              disabled={busy}
            >
              {busy ? "验证中…" : "开始授权"}
              <Icon name="arrow" size={16} />
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="form-intro">
            {method === "sms"
              ? "短信验证码已发送到默认手机号。"
              : "验证码已发送到受信任设备。"}{" "}
            也可以切换验证方式。
          </p>
          <div className="wizard-methods">
            <button
              type="button"
              className={`button ${method === "device" ? "primary" : "secondary"}`}
              onClick={() => setMethod("device")}
            >
              设备推送
            </button>
            <button
              type="button"
              className={`button ${method === "sms" ? "primary" : "secondary"}`}
              onClick={() => void switchSMS()}
              disabled={busy}
            >
              手机短信
            </button>
          </div>
          {method === "device" ? (
            <p className="field-help">
              请在 iPhone 或 Mac
              上确认登录；如果已经收到短信，也可以切换短信验证。
            </p>
          ) : (
            <>
              {phones.length > 0 && (
                <label className="field">
                  <span>接收手机号</span>
                  <select
                    value={phoneId}
                    onChange={(event) =>
                      void sendSMS(Number(event.target.value))
                    }
                  >
                    {phones.map((phone) => (
                      <option key={phone.id} value={phone.id}>
                        {phone.numberWithDialCode}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <p className="field-help">
                验证码已发送；切换手机号会重新发送短信。
              </p>
            </>
          )}
          <label className="field">
            <span>6 位验证码</span>
            <input
              value={code}
              onChange={(event) =>
                setCode(event.target.value.replace(/\D/g, "").slice(0, 6))
              }
              onKeyDown={(event) => {
                if (event.key === "Enter") void submitOTP();
              }}
              inputMode="numeric"
              maxLength={6}
              autoFocus
              placeholder="输入验证码"
            />
          </label>
          <div className="drawer-actions">
            <button
              type="button"
              className="button primary"
              onClick={() => void submitOTP()}
              disabled={busy}
            >
              {busy ? "提交中…" : "完成授权"}
              <Icon name="check" size={16} />
            </button>
          </div>
        </>
      )}
      {error && (
        <div className="form-error" role="alert">
          <Icon name="alert" size={16} />
          {error}
        </div>
      )}
    </div>
  );
}
