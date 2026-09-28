import { useState } from "react";
import { Plus, Search, SlidersHorizontal, ArrowUpRight } from "lucide-react";
import { Avatar, Empty } from "../../components/ui";
import { matches, date, type Workspace, type User } from "../../domain";
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
  const [filter, setFilter] = useState("all");
  const [limit, setLimit] = useState(12);
  const list = data.customers.filter(
    (c) =>
      (!portfolio || c.ownerId === user.id) &&
      matches(c, q) &&
      (filter === "all" || c.ownership === filter),
  );
  return (
    <>
      <div className="page-heading">
        <div>
          <span className="eyebrow">OAMENI, NU DOAR NUMERE</span>
          <h1>{portfolio ? "Portofoliul meu" : "Clienții magazinului"}</h1>
          <p>{list.length} relații. Fiecare cu propria poveste.</p>
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
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <SlidersHorizontal size={18} />
        <select
          aria-label="Responsabilitate"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        >
          <option value="all">Toți clienții</option>
          <option value="owned">Alocați</option>
          <option value="pool">În portofoliul magazinului</option>
          <option value="unassigned">Fără urmărire activă</option>
        </select>
      </div>
      <div className="customer-grid">
        {list.slice(0, limit).map((c) => (
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
              {c.tags.map((t) => (
                <span key={t} className="tag">
                  {t}
                </span>
              ))}
            </div>
            <footer>
              <span>
                {c.ownerId
                  ? data.users
                      .find((u) => u.id === c.ownerId)
                      ?.name.split(" ")[0]
                  : c.ownership === "pool"
                    ? "Magazin"
                    : "Fără urmărire"}
              </span>
              <span>{date(c.updatedAt)}</span>
            </footer>
          </button>
        ))}
      </div>
      {!list.length && <Empty>Nu am găsit clienți pentru aceste filtre.</Empty>}
      {list.length > limit && (
        <button className="button" onClick={() => setLimit(limit + 12)}>
          Arată mai mulți
        </button>
      )}
    </>
  );
}
