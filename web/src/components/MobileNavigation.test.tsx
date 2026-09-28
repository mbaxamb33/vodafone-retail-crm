// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import MobileNavigation from "./MobileNavigation";

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function () {
      this.setAttribute("open", "");
    },
  });
  Object.defineProperty(HTMLDialogElement.prototype, "close", {
    configurable: true,
    value: function () {
      this.removeAttribute("open");
    },
  });
});
afterEach(cleanup);
function mount(role: "manager" | "employee", open = true) {
  const onClose = vi.fn();
  const onCreate = vi.fn();
  const view = render(
    <MemoryRouter>
      <MobileNavigation
        user={{ id: "u", name: "Elena Demo", role, storeId: "s" }}
        storeName="Magazin demo"
        open={open}
        onClose={onClose}
        onToggle={vi.fn()}
        onCreate={onCreate}
        onLogout={vi.fn()}
      />
    </MemoryRouter>,
  );
  return { ...view, onClose, onCreate };
}
describe("phone navigation", () => {
  it("offers manager pages and closes the sheet when navigating", async () => {
    const { onClose, unmount } = mount("manager");
    expect(document.body.style.overflow).toBe("hidden");
    expect(screen.getByRole("link", { name: /Echipa mea/ })).toBeTruthy();
    await userEvent.click(
      screen.getByRole("link", { name: /Privire de ansamblu/ }),
    );
    expect(onClose).toHaveBeenCalledOnce();
    unmount();
    expect(document.body.style.overflow).toBe("");
  });
  it("keeps employee navigation focused on their own work", () => {
    mount("employee");
    expect(
      screen.queryByRole("link", { name: /Privire de ansamblu/ }),
    ).toBeNull();
    expect(screen.queryByRole("link", { name: /Echipa mea/ })).toBeNull();
    expect(screen.getByRole("link", { name: /Portofoliul meu/ })).toBeTruthy();
  });
  it("opens customer creation directly from the bottom navigation", async () => {
    const { onCreate } = mount("employee", false);
    await userEvent.click(
      screen.getByRole("button", { name: "Adaugă un client nou" }),
    );
    expect(onCreate).toHaveBeenCalledOnce();
  });
});
