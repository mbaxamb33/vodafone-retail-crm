import { useState } from "react";
import { Plus, Search, SlidersHorizontal, ArrowUpRight } from "lucide-react";
import { Avatar, Empty } from "../../components/ui";
import { date, lastInteraction, type Workspace, type User } from "../../domain";
import { useCustomerPages, useDebounced } from "../../hooks";

export default function CustomerList({
  data,
  user,
  portfolio,
  onOpen,
  onCreate,
}: {
  data: Workspace;
  user: User;
  portfolio: boolean;
  onOpen: (id: string) => void;
  onCreate: () => void;
}) {
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState("recent");
  const query = useDebounced(q.trim());
  const list = useCustomerPages({
    q: query,
    owner: portfolio ? "me" : "",
    ownership: portfolio ? "" : filter,
    sort,
  });
  const items = list.data?.pages.flatMap((p) => p.items) ?? [];
  const total = list.data?.pages[0]?.total ?? 0;
  const ownerName = (id: string) =>
    data.users.find((u) => u.id === id)?.name.split(" ")[0] ??
    (id === user.id ? user.name.split(" ")[0] : "Coleg");
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">OAMENI, NU DOAR NUMERE</span>
          <h1>{portfolio ? "Portofoliul meu" : "Clienții magazinului"}</h1>
          <p>
            {list.isPending ? "Se încarcă…" : `${total} relații.`} Fiecare cu
            propria poveste.
          </p>
        </div>
        <button className="button primary" onClick={onCreate}>
          <Plus size={18} />
          Client nou
        </button>
      </div>
      <div className="filter-bar">
        <div className="input-search">
          <Search size={19} />
          <input
            placeholder="Nume sau număr de telefon"
            aria-label="Filtrează clienții"
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
            }}
          />
        </div>
        <SlidersHorizontal size={18} />
        {!portfolio && (
          <select
            aria-label="Responsabilitate"
            value={filter}
            onChange={(e) => {
              setFilter(e.target.value);
            }}
          >
            <option value="">Toți clienții</option>
            <option value="owned">Alocați</option>
            <option value="pool">În portofoliul magazinului</option>
            <option value="unassigned">Fără urmărire activă</option>
          </select>
        )}
        <select
          aria-label="Ordonează"
          value={sort}
          onChange={(e) => setSort(e.target.value)}
        >
          <option value="recent">Interacțiune recentă</option>
          <option value="followup">Următorul follow-up</option>
          <option value="newest">Clienți noi</option>
          <option value="name">Nume</option>
        </select>
        {(q || filter) && (
          <button
            className="button subtle"
            onClick={() => {
              setQ("");
              setFilter("");
            }}
          >
            Resetează filtrele
          </button>
        )}
      </div>
      {list.isError && (
        <div className="error-panel">
          <p className="error">{list.error.message}</p>
          <button className="button" onClick={() => list.refetch()}>
            Încearcă din nou
          </button>
        </div>
      )}
      {list.isPending ? (
        <div className="skeleton">
          <div />
          <div />
        </div>
      ) : (
        <div className="customer-grid">
          {items.map((c) => (
            <button
              className="customer-card"
              onClick={() => onOpen(c.id)}
              key={c.id}
            >
              <div className="customer-card-top">
                <Avatar name={c.name} />
                <ArrowUpRight size={20} />
              </div>
              <h3>{c.name}</h3>
              <p>{c.phone}</p>
              <div className="tags">
                {c.status === "archived" && (
                  <span className="tag">Arhivat</span>
                )}
                {c.nextFollowUpDue && (
                  <span className="tag">
                    Revenire {date(c.nextFollowUpDue)}
                  </span>
                )}
                {c.tags.map((t) => (
                  <span key={t} className="tag">
                    {t}
                  </span>
                ))}
              </div>
              <footer>
                <span>
                  {c.ownerId
                    ? ownerName(c.ownerId)
                    : c.ownership === "pool"
                      ? "Magazin"
                      : "Fără urmărire"}
                </span>
                <span>{date(lastInteraction(c))}</span>
              </footer>
            </button>
          ))}
        </div>
      )}
      {list.isSuccess && !items.length && (
        <Empty>Nu am găsit clienți pentru aceste filtre.</Empty>
      )}
      {list.hasNextPage && (
        <button
          className="button"
          disabled={list.isFetchingNextPage}
          onClick={() => list.fetchNextPage()}
        >
          {list.isFetchingNextPage ? "Se încarcă…" : "Arată mai mulți"}
        </button>
      )}
    </>
  );
}
