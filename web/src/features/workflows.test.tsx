// @vitest-environment jsdom
import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import type { ReactNode } from "react";
import { api, ApiError } from "../api";
import { workspaceSchema, type User } from "../domain";
import Manager from "./manager/Manager";
import Team from "./manager/Team";
import VisitForm from "./customers/VisitForm";
import CreateCustomer from "./customers/CreateCustomer";
import OwnershipEditor from "./customers/OwnershipEditor";
import FollowUpForm from "./followups/FollowUpForm";

vi.mock("../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api")>()),
  api: vi.fn(),
}));
const manager: User = {
  id: "m",
  name: "Manager Demo",
  role: "manager",
  storeId: "s",
};
const employee: User = {
  id: "e",
  name: "Consultant Demo",
  role: "employee",
  storeId: "s",
};
const data = workspaceSchema.parse({
  users: [employee, manager],
  customers: [
    {
      id: "c",
      name: "Client Demo",
      phone: "+40722000999",
      storeId: "s",
      ownerId: "e",
      ownership: "owned",
      tags: [],
      createdAt: "2026-09-01T12:00:00Z",
      updatedAt: "2026-09-28T12:00:00Z",
    },
  ],
  followUps: [],
  opportunities: [],
});
function mount(node: ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <MemoryRouter>{node}</MemoryRouter>
    </QueryClientProvider>,
  );
}
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});
function dialogs() {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function () {
      this.setAttribute("open", "");
    },
  });
}
const employeeReport = {
  employeeId: "e",
  name: "Consultant Demo",
  role: "employee",
  visits: 0,
  customersHandled: 0,
  newCustomers: 0,
  opportunitiesCreated: 0,
  offers: 0,
  contracts: 0,
  portfolioCustomers: 1,
  activeOpportunities: 0,
  openFollowUps: 0,
  overdueFollowUps: 0,
};
const report = {
  from: "",
  to: "",
  summary: {
    visits: 0,
    customersHandled: 0,
    newCustomers: 0,
    opportunitiesCreated: 0,
    offers: 0,
    contracts: 0,
    lost: 0,
    poolCustomers: 0,
    activeOpportunities: 0,
    followUpsDueToday: 0,
    overdueFollowUps: 0,
  },
  funnel: [{ step: -1, label: "Vizite în magazin", count: 0, rate: 1 }],
  stepIncidence: [{ step: 0, label: "Welcome", count: 0 }],
  employees: [employeeReport],
};
// Routes mocked API calls by path, as the manager screens load several resources.
function mockManagerApi() {
  vi.mocked(api).mockImplementation(async (path: string) => {
    if (path.startsWith("manager/dashboard")) return report;
    if (path.startsWith("manager/employees/"))
      return {
        items: [],
        total: 0,
        offset: 0,
        limit: 100,
        summary: employeeReport,
      };
    if (path.startsWith("customers?"))
      return { items: data.customers, total: 1, offset: 0, limit: 24 };
    throw new Error("unexpected " + path);
  });
}
describe("retail workflows", () => {
  it("gives the overview and team different jobs", async () => {
    mockManagerApi();
    const view = mount(<Manager data={data} onOpen={vi.fn()} />);
    expect(
      screen.getByRole("heading", { name: "Ce are nevoie de atenția ta?" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /Oportunități blocate/ }),
    ).toBeTruthy();
    await screen.findByRole("heading", { name: "Pași parcurși în vizite" });
    expect(screen.queryByRole("button", { name: "Reasignează" })).toBeNull();
    expect(
      screen
        .getByRole("link", { name: /Consultant Demo/ })
        .getAttribute("href"),
    ).toBe("/team?employee=e");
    view.unmount();
    mount(<Team data={data} user={manager} onOpen={vi.fn()} />);
    expect(
      screen.getByRole("heading", { name: "Oamenii din spatele relațiilor." }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("button", { name: "Reasignează" }),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Activitate" }));
    await screen.findByText(/0 vizite/);
    expect(
      screen.queryByRole("heading", { name: "Pași parcurși în vizite" }),
    ).toBeNull();
  });
  it("records a colleague visit without claiming ownership", async () => {
    dialogs();
    vi.mocked(api).mockImplementation(async (path: string) =>
      path === "catalog"
        ? {
            visitReasons: [{ code: "support", label: "Service / suport" }],
            nextActions: [],
            productCategories: [],
          }
        : { ok: true },
    );
    const saved = vi.fn();
    mount(
      <VisitForm
        customer={data.customers[0]}
        user={manager}
        users={data.users}
        onClose={vi.fn()}
        onSaved={saved}
      />,
    );
    expect(screen.getByText("Responsabil: Consultant Demo")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Prezentare" }));
    await userEvent.click(
      screen.getByRole("button", { name: "Salvează vizita" }),
    );
    await waitFor(() => expect(saved).toHaveBeenCalledOnce());
    expect(api).toHaveBeenCalledWith(
      "customers/c/visits",
      expect.objectContaining({
        ownership: "keep",
        steps: [0, 5],
        reasonCode: "support",
        opportunities: [],
      }),
    );
  });
  it("prefills the searched phone when creating a customer", () => {
    dialogs();
    mount(
      <CreateCustomer
        initialPhone="0722 000 999"
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />,
    );
    expect(
      (
        screen.getByRole("textbox", {
          name: "Număr de telefon",
        }) as HTMLInputElement
      ).value,
    ).toBe("0722 000 999");
  });
  it("shows server validation next to the field", async () => {
    dialogs();
    vi.mocked(api).mockRejectedValue(
      new ApiError(
        422,
        "VALIDATION_FAILED",
        "Verifică datele introduse.",
        "r",
        {
          phone: "Introdu un număr de telefon valid.",
        },
      ),
    );
    mount(
      <CreateCustomer
        initialPhone="0000 0000"
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />,
    );
    await userEvent.type(
      screen.getByRole("textbox", { name: "Nume client" }),
      "Client Test",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Adaugă clientul" }),
    );
    expect(
      await screen.findByText("Introdu un număr de telefon valid."),
    ).toBeTruthy();
    expect(screen.queryByText("Verifică datele introduse.")).toBeNull();
  });
  it("keeps a rejected ownership change editable and shows the error", async () => {
    vi.mocked(api).mockRejectedValue(new Error("Nu ai permisiunea necesară."));
    const pool = {
      ...data.customers[0],
      ownerId: "",
      ownership: "pool" as const,
    };
    mount(
      <OwnershipEditor customer={pool} user={employee} users={data.users} />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Alocă un responsabil" }),
    );
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Responsabil nou" }),
      "e",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Salvează responsabilul" }),
    );
    await screen.findByRole("alert");
    expect(screen.getByRole("alert").textContent).toBe(
      "Nu ai permisiunea necesară.",
    );
    expect(screen.getByRole("combobox")).toBeTruthy();
    expect(api).toHaveBeenCalledWith("customers/c/ownership", {
      ownership: "owned",
      ownerId: "e",
    });
  });
  it("reschedules an unreachable customer with a next date", async () => {
    dialogs();
    vi.mocked(api).mockResolvedValue({ ok: true });
    const close = vi.fn();
    mount(
      <FollowUpForm
        customerId="c"
        customerName="Client Demo"
        user={employee}
        users={data.users}
        followUp={{
          id: "f",
          customerId: "c",
          employeeId: "e",
          type: "Call",
          due: "2026-10-02",
          status: "open",
        }}
        onClose={close}
      />,
    );
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Rezultatul revenirii" }),
      "unreachable",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Salvează următorul pas" }),
    );
    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(api).toHaveBeenCalledWith(
      "follow-ups/f",
      { status: "unreachable", due: "2026-10-02" },
      "PATCH",
    );
  });
});
