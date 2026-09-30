import Experience from "./features/experience/Experience";
import MobileNavigation from "./components/MobileNavigation";
import Brand, { BrandMark } from "./components/Brand";
import { Avatar, Empty } from "./components/ui";
import Manager from "./features/manager/Manager";
import Team from "./features/manager/Team";
import VisitForm from "./features/customers/VisitForm";
import CreateCustomer from "./features/customers/CreateCustomer";
import Profile from "./features/customers/Profile";
import Pipeline from "./features/opportunities/Pipeline";
import FollowUps from "./features/followups/FollowUps";
import CustomerList from "./features/customers/CustomerList";
import Notifications from "./features/notifications/Notifications";
import { useEffect, useState, useRef } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpRight,
  ArrowRight,
  Check,
  ChevronRight,
  Clock,
  Home,
  Users,
  BriefcaseBusiness,
  ChartNoAxesCombined,
  CalendarDays,
  Search,
  Plus,
  X,
  LogOut,
  CheckCheck,
  Store,
  TrendingUp,
  MessageSquare,
  ShieldCheck,
  Sun,
} from "lucide-react";
import { api } from "./api";
import { useCustomers, useDebounced } from "./hooks";
import {
  meSchema,
  workspaceSchema,
  stages,
  date,
  today,
  lastInteraction,
  type Customer,
} from "./domain";
import type { z } from "zod";

const nav = [
  ["/", "Astăzi", Home],
  ["/customers", "Clienți", Users],
  ["/store-pool", "Clienții magazinului", Store],
  ["/portfolio", "Portofoliul meu", BriefcaseBusiness],
  ["/pipeline", "Oportunități", ChartNoAxesCombined],
  ["/followups", "Follow-up-uri", CalendarDays],
] as const;
export default function App() {
  const qc = useQueryClient();
  const searchRef = useRef<HTMLDivElement>(null);
  const [me, setMe] = useState<z.infer<typeof meSchema> | null>(null);
  const user = me?.user ?? null;
  const [boot, setBoot] = useState(true);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounced(search.trim());
  const [initialPhone, setInitialPhone] = useState("");
  const [visit, setVisit] = useState<Customer | null>(null);
  const [create, setCreate] = useState(false);
  const [mobile, setMobile] = useState(false);
  const [toast, setToast] = useState("");
  const location = useLocation();
  const navigate = useNavigate();
  const selected =
    location.pathname.match(/^\/customers\/([^/]+)$/)?.[1] ?? null;
  const setSelected = (id: string | null) =>
    navigate(id ? `/customers/${id}` : "/customers");
  useEffect(() => {
    api("auth/me")
      .then((v) => setMe(meSchema.parse(v)))
      .catch(() => {})
      .finally(() => setBoot(false));
    const expired = () => {
      setMe(null);
      qc.clear();
    };
    window.addEventListener("session-expired", expired);
    return () => window.removeEventListener("session-expired", expired);
  }, [qc]);
  useEffect(() => {
    if (toast) {
      const timer = setTimeout(() => setToast(""), 4000);
      return () => clearTimeout(timer);
    }
  }, [toast]);
  useEffect(() => {
    const dismiss = (event: PointerEvent) => {
      if (!searchRef.current?.contains(event.target as Node)) setSearch("");
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSearch("");
    };
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", dismiss);
      document.removeEventListener("keydown", escape);
    };
  }, []);
  useEffect(() => {
    window.scrollTo({ top: 0, behavior: "instant" });
  }, [location.pathname]);
  const login = useMutation({
    mutationFn: () => api("auth/login", { email, password }),
    onSuccess: (v) => {
      qc.clear();
      setPassword("");
      setMe(meSchema.parse(v));
    },
  });
  const ws = useQuery({
    queryKey: ["workspace", user?.id],
    queryFn: async () => workspaceSchema.parse(await api("workspace")),
    enabled: !!user,
  });
  const searchResults = useCustomers(
    { q: debouncedSearch, limit: 6 },
    !!user && debouncedSearch.length >= 2,
  );
  const recent = useCustomers({ sort: "recent", limit: 3 }, !!user);
  const refresh = (message: string) => {
    void qc.invalidateQueries();
    setToast(message);
  };
  const complete = useMutation({
    mutationFn: (id: string) =>
      api("follow-ups/" + id, { status: "done" }, "PATCH"),
    onSuccess: () => refresh("Follow-up finalizat. Bravo!"),
    onError: (e) => setToast(e.message),
  });
  if (boot)
    return (
      <div className="boot">
        <BrandMark />
        <p>Se pregătește spațiul tău de lucru…</p>
      </div>
    );
  if (!user)
    return (
      <div className="login">
        <section className="login-story">
          <Brand />
          <div>
            <span className="eyebrow">FIECARE CONVERSAȚIE CONTEAZĂ</span>
            <h1>
              Mai aproape.
              <br />
              Cu fiecare vizită.
            </h1>
            <p>
              Oameni, conversații și oportunități.
              <br />
              Toate într-un singur loc.
            </p>
          </div>
          <small>Relații care merg mai departe.</small>
        </section>
        <section className="login-form">
          <div className="login-card">
            <span className="eyebrow">BUN VENIT ÎN ECHIPĂ</span>
            <h2>O zi bună începe aici.</h2>
            <p>Intră în spațiul tău de lucru.</p>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                login.mutate();
              }}
            >
              <label htmlFor="email">Utilizator</label>
              <input
                id="email"
                type="text"
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                placeholder="prenume.nume"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <label htmlFor="password">Parolă</label>
              <input
                id="password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
              <button
                className="button primary full"
                disabled={login.isPending}
              >
                {login.isPending ? "Se conectează…" : "Intră în aplicație"}
                <ArrowRight size={18} />
              </button>
            </form>
            {login.error && (
              <p className="error" role="alert">
                {login.error.message}
              </p>
            )}
            <div className="demo-note">
              <ShieldCheck size={18} />
              <span>
                Datele clienților sunt confidențiale. Nu îți împărtăși contul și
                deconectează-te când pleci de la calculator.
              </span>
            </div>
          </div>
        </section>
      </div>
    );
  const data = ws.data;
  const customers = data?.customers ?? [];
  const open =
    data?.followUps.filter(
      (f) => f.status !== "done" && f.employeeId === user.id,
    ) ?? [];
  const overdue = open.filter((f) => f.due < today());
  const due = open.filter((f) => f.due === today());
  const opportunities =
    data?.opportunities.filter(
      (o) =>
        o.stage !== "won" && o.stage !== "lost" && o.employeeId === user.id,
    ) ?? [];
  const mine = customers.filter((c) => c.ownerId === user.id);
  const page = location.pathname;
  const showCustomer = (id: string) => {
    setSelected(id);
    setSearch("");
  };
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        Sari la conținut
      </a>
      <aside className="sidebar">
        <NavLink to="/" aria-label="Bright Signa — Acasă">
          <Brand />
        </NavLink>
        <div className="store-label">
          <Store size={17} />
          <div>
            {me?.store.name}
            <small>Magazinul tău</small>
          </div>
          <span className="live-dot" />
        </div>
        <span className="nav-label">SPAȚIUL MEU</span>
        <nav>
          {nav.map(([path, label, Icon]) => (
            <NavLink key={path} to={path} end onClick={() => setMobile(false)}>
              <Icon size={20} />
              {label}
              {path === "/followups" && open.length > 0 && (
                <span className="nav-count">{open.length}</span>
              )}
            </NavLink>
          ))}
        </nav>
        {user.role === "manager" && (
          <>
            <span className="nav-label manager-label">MAGAZIN</span>
            <nav>
              <NavLink to="/manager">
                <TrendingUp size={20} />
                Privire de ansamblu
              </NavLink>
              <NavLink to="/team">
                <Users size={20} />
                Echipa mea
              </NavLink>
            </nav>
          </>
        )}
        <div className="sidebar-bottom">
          <div className="relationship">
            <span className="mini-spark">✳</span>
            <strong>
              Relațiile bune încep
              <br />
              cu o conversație.
            </strong>
            <span>Fă-o să conteze.</span>
          </div>
          <div className="user">
            <Avatar name={user.name} small />
            <div>
              <strong>{user.name}</strong>
              <small>
                {user.role === "manager"
                  ? "Manager magazin"
                  : "Consultant vânzări"}
              </small>
            </div>
            <button
              className="icon-button"
              aria-label="Deconectare"
              onClick={async () => {
                await api("auth/logout", {}).catch(() => {});
                setMe(null);
                qc.clear();
              }}
            >
              <LogOut size={17} />
            </button>
          </div>
        </div>
      </aside>
      <div className="main-shell">
        <header className="topbar">
          <NavLink
            to="/"
            className="mobile-header-brand"
            aria-label="Bright Signa — Acasă"
          >
            <Brand />
          </NavLink>
          <div className="header-context">
            <span>SPAȚIUL TĂU</span>
            <strong>
              {selected
                ? "Relația cu clientul"
                : page === "/manager"
                  ? "Privire de ansamblu"
                  : page === "/team"
                    ? "Echipa mea"
                    : (nav.find(([path]) => path === page)?.[1] ??
                      "Bright Signa")}
            </strong>
          </div>
          <div className="global-search" ref={searchRef}>
            <Search size={19} />
            <input
              aria-label="Caută un client"
              placeholder="Caută un nume sau un telefon…"
              inputMode="search"
              autoComplete="off"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <span className="search-hint">Caută & conectează</span>
            {search && (
              <div className="search-results">
                {search.trim().length < 2 ? (
                  <p>Continuă să scrii numele sau numărul.</p>
                ) : searchResults.isPending ||
                  debouncedSearch !== search.trim() ? (
                  <p>Se caută…</p>
                ) : searchResults.isError ? (
                  <p role="alert">{searchResults.error.message}</p>
                ) : (
                  <>
                    {searchResults.data.items.map((c) => (
                      <div className="search-result-row" key={c.id}>
                        <button onClick={() => showCustomer(c.id)}>
                          <Avatar name={c.name || c.phone} small />
                          <div>
                            <strong>{c.name || c.phone}</strong>
                            <small>{c.phone}</small>
                          </div>
                          <ChevronRight size={17} />
                        </button>
                        <button
                          className="quick-visit-result"
                          aria-label={"Înregistrează vizita pentru " + c.name}
                          onClick={() => {
                            showCustomer(c.id);
                            setVisit(c);
                          }}
                        >
                          <Plus size={16} />
                          Vizită
                        </button>
                      </div>
                    ))}
                    {!searchResults.data.items.length && (
                      <p>Nu am găsit acest client.</p>
                    )}
                  </>
                )}
                <button
                  onClick={() => {
                    setInitialPhone(/^[+\d\s()-]+$/.test(search) ? search : "");
                    setCreate(true);
                    setSearch("");
                  }}
                >
                  <Plus size={18} />
                  Adaugă un client nou
                </button>
              </div>
            )}
          </div>
          <div className="topbar-right">
            <Notifications onOpen={showCustomer} />
            <Avatar name={user.name} small />
          </div>
        </header>
        <main id="main-content" tabIndex={-1}>
          {ws.isPending ? (
            <div className="skeleton">
              <div />
              <div />
              <div />
            </div>
          ) : ws.isError ? (
            <div className="empty">
              <h2>Nu am putut încărca datele.</h2>
              <button className="button" onClick={() => ws.refetch()}>
                Încearcă din nou
              </button>
            </div>
          ) : (
            data && (
              <>
                {page === "/" && (
                  <>
                    <div className="page-heading dashboard-hero">
                      <div>
                        <div className="eyebrow date-line">
                          <Sun size={15} />
                          {new Intl.DateTimeFormat("ro-RO", {
                            weekday: "long",
                            day: "numeric",
                            month: "long",
                            year: "numeric",
                            timeZone: "Europe/Bucharest",
                          }).format(new Date())}
                        </div>
                        <h1>
                          Bună, {user.name.split(" ")[0]}
                          <span className="greeting-dot">.</span>
                        </h1>
                        <p>O nouă zi. Noi conversații care contează.</p>
                      </div>
                      <button
                        className="button primary"
                        onClick={() => {
                          setInitialPhone("");
                          setCreate(true);
                        }}
                      >
                        <Plus size={18} />
                        Client nou
                      </button>
                      <div className="hero-art" aria-hidden="true">
                        <BrandMark />
                        <span className="hero-orbit orbit-one" />
                        <span className="hero-orbit orbit-two" />
                      </div>
                    </div>
                    <div className="stats-grid dashboard-stats">
                      {[
                        {
                          label: "Clienții mei",
                          value: mine.length,
                          icon: Users,
                          note: "Relații în grija ta",
                          to: "/portfolio",
                        },
                        {
                          label: "Oportunități active",
                          value: opportunities.length,
                          icon: TrendingUp,
                          note: "Conversații cu potențial",
                          to: "/pipeline",
                        },
                        {
                          label: "Follow-up-uri astăzi",
                          value: due.length,
                          icon: CalendarDays,
                          note: "Un moment bun să revii",
                          to: "/followups",
                        },
                        {
                          label: "Au nevoie de atenție",
                          value: overdue.length,
                          icon: Clock,
                          note: "Follow-up-uri restante",
                          to: "/followups",
                          attention: true,
                        },
                      ].map((m) => (
                        <button
                          className={
                            "stat-card " + (m.attention ? "attention" : "")
                          }
                          key={m.label}
                          onClick={() => navigate(m.to)}
                        >
                          <div>
                            <span>{m.label}</span>
                            <m.icon size={19} />
                          </div>
                          <strong>{m.value.toString().padStart(2, "0")}</strong>
                          <small>
                            {m.attention && <span className="red-dot" />}
                            {m.note}
                            <ArrowUpRight size={15} />
                          </small>
                        </button>
                      ))}
                    </div>
                    <Experience
                      onOpen={showCustomer}
                      data={data}
                      userId={user.id}
                    />
                    <div className="dashboard-columns">
                      <div>
                        <section className="card work-card">
                          <div className="section-heading">
                            <div>
                              <h2>
                                De făcut, cu grijă.{" "}
                                <span className="count-pill">
                                  {overdue.length + due.length}
                                </span>
                              </h2>
                              <p>
                                Conversații de continuat. Promisiuni de păstrat.
                              </p>
                            </div>
                            <NavLink to="/followups" className="text-link">
                              Vezi toate <ArrowRight size={16} />
                            </NavLink>
                          </div>
                          <div className="queue-label">
                            <span className="red-dot" />
                            DE REVENIT CU PRIORITATE{" "}
                            <span>{overdue.length} restante</span>
                          </div>
                          {[...overdue, ...due].slice(0, 5).map((f) => {
                            const c = customers.find(
                              (c) => c.id === f.customerId,
                            )!;
                            return (
                              <div className="task-row" key={f.id}>
                                <Avatar name={c.name || c.phone} />
                                <button
                                  className="task-main"
                                  onClick={() => showCustomer(c.id)}
                                >
                                  <strong>{c.name || c.phone}</strong>
                                  <span>
                                    {f.type}
                                    {f.notes && (
                                      <small className="reminder-context">
                                        {f.notes}
                                      </small>
                                    )}{" "}
                                    <span className="separator">·</span>{" "}
                                    {data.opportunities.find(
                                      (o) => o.customerId === c.id,
                                    )?.product ?? "Relație client"}
                                  </span>
                                </button>
                                <span
                                  className={
                                    "due-tag " + (f.due < today() ? "late" : "")
                                  }
                                >
                                  {f.due < today() ? date(f.due) : "Astăzi"}
                                </span>
                                <button
                                  className="circle-action"
                                  aria-label={
                                    "Finalizează follow-up pentru " + c.name
                                  }
                                  disabled={complete.isPending}
                                  onClick={() => complete.mutate(f.id)}
                                >
                                  <Check size={17} />
                                </button>
                              </div>
                            );
                          })}
                          {!overdue.length && !due.length && (
                            <Empty>Nu ai follow-up-uri pentru astăzi.</Empty>
                          )}
                          <div className="queue-footer">
                            <CheckCheck size={17} />
                            Fiecare revenire poate fi începutul unei
                            oportunități.
                          </div>
                        </section>
                        <section className="recent-section">
                          <div className="section-heading">
                            <h2>Conversații recente</h2>
                            <NavLink to="/customers" className="text-link">
                              Toți clienții <ArrowRight size={16} />
                            </NavLink>
                          </div>
                          <div className="recent-grid">
                            {(recent.data?.items ?? []).map((c) => (
                              <button
                                className="recent-card"
                                key={c.id}
                                onClick={() => showCustomer(c.id)}
                              >
                                <div>
                                  <Avatar name={c.name || c.phone} />
                                  <ArrowUpRight size={17} />
                                </div>
                                <h3>{c.name || c.phone}</h3>
                                <p>{c.phone}</p>
                                <span className="tag">
                                  {c.tags[0] ?? "Client nou"}
                                </span>
                                <footer>
                                  Ultima interacțiune
                                  <span>{date(lastInteraction(c))}</span>
                                </footer>
                              </button>
                            ))}
                          </div>
                        </section>
                      </div>
                      <div>
                        <section className="card pipeline-summary">
                          <div className="section-heading">
                            <h2>Oportunitățile tale</h2>
                            <ChartNoAxesCombined size={19} />
                          </div>
                          <p>De la primul interes la un nou început.</p>
                          {[
                            "identified",
                            "verification",
                            "presentation",
                            "offer",
                            "waiting",
                          ].map((s, i) => {
                            const count = opportunities.filter(
                              (o) => o.stage === s,
                            ).length;
                            return (
                              <button
                                key={s}
                                className="pipeline-line"
                                onClick={() => navigate("/pipeline")}
                              >
                                <span className={"stage-square s" + i} />
                                <span>{stages[s]}</span>
                                <strong>{count}</strong>
                                <div className="bar">
                                  <i
                                    style={{
                                      width: `${Math.max(8, (count / Math.max(opportunities.length, 1)) * 100)}%`,
                                    }}
                                  />
                                </div>
                              </button>
                            );
                          })}
                          <NavLink
                            className="button full subtle"
                            to="/pipeline"
                          >
                            Deschide oportunitățile <ArrowRight size={16} />
                          </NavLink>
                        </section>
                        <section className="focus-card">
                          <div className="focus-icon">
                            <MessageSquare size={23} />
                          </div>
                          <span className="eyebrow">
                            MAI MULT DECÂT O VÂNZARE
                          </span>
                          <h2>
                            Ține minte detaliile.
                            <br />
                            Construiește relația.
                          </h2>
                          <p>
                            O notă după fiecare vizită te ajută să reiei
                            conversația exact de unde a rămas.
                          </p>
                          <button onClick={() => navigate("/customers")}>
                            Găsește un client <ArrowRight size={17} />
                          </button>
                          <div className="decorative-ring" />
                        </section>
                      </div>
                    </div>
                  </>
                )}
                {(page === "/customers" ||
                  page === "/portfolio" ||
                  page === "/store-pool") && (
                  <CustomerList
                    data={data}
                    user={user}
                    key={page}
                    portfolio={page === "/portfolio"}
                    pool={page === "/store-pool"}
                    onOpen={showCustomer}
                    onCreate={() => {
                      setInitialPhone("");
                      setCreate(true);
                    }}
                  />
                )}
                {page === "/followups" && (
                  <FollowUps
                    data={data}
                    user={user}
                    onOpen={showCustomer}
                    onComplete={(id) => complete.mutate(id)}
                    pending={complete.isPending}
                  />
                )}
                {page === "/pipeline" && (
                  <Pipeline
                    data={data}
                    user={user}
                    onOpen={showCustomer}
                    refresh={refresh}
                  />
                )}
                {(page === "/manager" || page === "/team") &&
                  (user.role === "manager" ? (
                    page === "/manager" ? (
                      <Manager data={data} onOpen={showCustomer} />
                    ) : (
                      <Team data={data} user={user} onOpen={showCustomer} />
                    )
                  ) : (
                    <Empty>Acest spațiu este disponibil managerilor.</Empty>
                  ))}
                {selected && (
                  <Profile
                    key={selected}
                    id={selected}
                    data={data}
                    user={user}
                    onClose={() => setSelected(null)}
                    onVisit={setVisit}
                  />
                )}
                {!selected &&
                  ![
                    "/",
                    "/customers",
                    "/portfolio",
                    "/followups",
                    "/pipeline",
                    "/manager",
                    "/team",
                  ].includes(page) && (
                    <Empty>
                      Pagina nu a fost găsită. Alege o secțiune din meniu.
                    </Empty>
                  )}
              </>
            )
          )}
        </main>
        <footer className="app-footer">
          <Brand compact />
          <span>Relații care contează. În fiecare zi.</span>
          <span>Uz intern · Date confidențiale</span>
        </footer>
      </div>
      <MobileNavigation
        user={user}
        storeName={me?.store.name ?? "Magazinul tău"}
        open={mobile}
        onToggle={() => setMobile((v) => !v)}
        onClose={() => setMobile(false)}
        onCreate={() => {
          setInitialPhone("");
          setCreate(true);
        }}
        onLogout={() => {
          void api("auth/logout", {})
            .catch(() => {})
            .finally(() => {
              setMe(null);
              qc.clear();
            });
        }}
      />
      {visit && (
        <VisitForm
          customer={visit}
          user={user}
          users={data?.users ?? []}
          onClose={() => setVisit(null)}
          onSaved={() => {
            setVisit(null);
            setSelected(visit.id);
            refresh("Vizita a fost salvată.");
          }}
        />
      )}
      {create && (
        <CreateCustomer
          initialPhone={initialPhone}
          onClose={() => setCreate(false)}
          onOpenExisting={(id) => {
            setCreate(false);
            showCustomer(id);
          }}
          onSaved={(c) => {
            setCreate(false);
            setSelected(c.id);
            refresh("Clientul a fost adăugat.");
          }}
        />
      )}
      {toast && (
        <div role="status" className="toast">
          <Check size={18} />
          {toast}
          <button aria-label="Închide notificarea" onClick={() => setToast("")}>
            <X size={16} />
          </button>
        </div>
      )}
    </div>
  );
}
