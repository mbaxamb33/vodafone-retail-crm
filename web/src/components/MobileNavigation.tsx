import { NavLink, useLocation } from "react-router-dom";
import {
  Home,
  Users,
  Plus,
  CalendarDays,
  Grid2X2,
  BriefcaseBusiness,
  ChartNoAxesCombined,
  TrendingUp,
  ArrowUpRight,
  LogOut,
  Store,
  type LucideIcon,
} from "lucide-react";
import { Modal, Avatar } from "./ui";
import type { User } from "../domain";
export default function MobileNavigation({
  user,
  storeName,
  open,
  onToggle,
  onClose,
  onCreate,
  onLogout,
}: {
  user: User;
  storeName: string;
  open: boolean;
  onToggle: () => void;
  onClose: () => void;
  onCreate: () => void;
  onLogout: () => void;
}) {
  const { pathname } = useLocation();
  const moreActive = [
    "/portfolio",
    "/store-pool",
    "/pipeline",
    "/manager",
    "/team",
  ].some((path) => pathname.startsWith(path));
  const links: [string, string, string, LucideIcon][] = [
    [
      "/store-pool",
      "Clienții magazinului",
      "Relații disponibile pentru preluare",
      Store,
    ],
    [
      "/portfolio",
      "Portofoliul meu",
      "Relațiile în grija ta",
      BriefcaseBusiness,
    ],
    [
      "/pipeline",
      "Oportunități",
      "Următorul pas spre o vânzare",
      ChartNoAxesCombined,
    ],
    ...(user.role === "manager"
      ? ([
          [
            "/manager",
            "Privire de ansamblu",
            "Prioritățile magazinului",
            TrendingUp,
          ],
          ["/team", "Echipa mea", "Oamenii din spatele relațiilor", Users],
        ] as [string, string, string, LucideIcon][])
      : []),
  ];
  return (
    <>
      <nav className="mobile-bottom-nav" aria-label="Navigare pe telefon">
        <NavLink to="/" end onClick={onClose}>
          <Home size={21} />
          <span>Astăzi</span>
        </NavLink>
        <NavLink to="/customers" onClick={onClose}>
          <Users size={21} />
          <span>Clienți</span>
        </NavLink>
        <button
          className="mobile-create"
          aria-label="Adaugă un client nou"
          onClick={() => {
            onClose();
            onCreate();
          }}
        >
          <span className="mobile-create-icon">
            <Plus size={25} />
          </span>
          <span>Client nou</span>
        </button>
        <NavLink to="/followups" onClick={onClose}>
          <CalendarDays size={21} />
          <span>Reveniri</span>
        </NavLink>
        <button
          aria-label="Mai mult"
          aria-expanded={open}
          className={open || moreActive ? "active" : ""}
          onClick={onToggle}
        >
          <Grid2X2 size={21} />
          <span>Mai mult</span>
        </button>
      </nav>
      {open && (
        <Modal
          title="Spațiul tău."
          onClose={onClose}
          className="navigation-sheet"
        >
          <div className="mobile-account">
            <Avatar name={user.name} />
            <div>
              <strong>{user.name}</strong>
              <span>
                {user.role === "manager"
                  ? "Manager magazin"
                  : "Consultant vânzări"}
              </span>
            </div>
          </div>
          <div className="mobile-store">
            <Store size={17} />
            {storeName}
          </div>
          <nav className="mobile-menu-links" aria-label="Toate secțiunile">
            {links.map(([to, label, description, Icon]) => (
              <NavLink to={to} key={to} onClick={onClose}>
                <span className="menu-link-icon">
                  <Icon size={22} />
                </span>
                <span>
                  <strong>{label}</strong>
                  <small>{description}</small>
                </span>
                <ArrowUpRight size={19} />
              </NavLink>
            ))}
          </nav>
          <button
            className="button logout-button"
            onClick={() => {
              onClose();
              onLogout();
            }}
          >
            <LogOut size={18} />
            Deconectare
          </button>
        </Modal>
      )}
    </>
  );
}
