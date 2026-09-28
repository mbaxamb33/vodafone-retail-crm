import { nextOpportunityAction, staleOpportunity, dateOffset } from "./domain";
import { describe, it, expect } from "vitest";
import {
  normalizePhone,
  matches,
  customerSchema,
  workspaceSchema,
} from "./domain";
describe("customer lookup", () => {
  it.each([
    "0722 345 678",
    "+40 722 345 678",
    "0040722345678",
    "(0722) 345-678",
  ])("normalizes %s", (phone) => {
    expect(normalizePhone(phone)).toBe("+40722345678");
  });
  it("matches formatted numbers and names", () => {
    const c = customerSchema.parse({
      id: "1",
      name: "Ioana Marinescu",
      phone: "+40722345678",
      storeId: "s",
      ownerId: "",
      ownership: "pool",
      tags: [],
      createdAt: "2026-09-28",
      updatedAt: "2026-09-28",
    });
    expect(matches(c, "0722 345 678")).toBe(true);
    expect(matches(c, "IOANA")).toBe(true);
    expect(matches(c, "Andrei")).toBe(false);
  });
  it("rejects malformed API data", () => {
    expect(workspaceSchema.safeParse({ customers: [{}] }).success).toBe(false);
  });
});

describe("opportunity next actions", () => {
  it("does not infer links from other tasks for the same customer", () => {
    const o = {
      id: "o",
      customerId: "c",
      employeeId: "e",
      product: "Internet",
      stage: "offer",
      createdAt: "2026-09-01T00:00:00Z",
    };
    const f = {
      id: "f",
      customerId: "c",
      employeeId: "e",
      type: "Call",
      due: "2026-10-01",
      status: "open" as const,
    };
    expect(nextOpportunityAction(o, [f])).toBeUndefined();
    expect(nextOpportunityAction(o, [{ ...f, opportunityId: "o" }])?.id).toBe(
      "f",
    );
    expect(
      nextOpportunityAction(o, [{ ...f, opportunityId: "o", status: "done" }]),
    ).toBeUndefined();
  });
  it("flags stale active stages and respects the last stage change", () => {
    const now = Date.parse("2026-09-28T12:00:00Z");
    const o = {
      id: "o",
      customerId: "c",
      employeeId: "e",
      product: "Internet",
      stage: "offer",
      createdAt: "2026-09-01T00:00:00Z",
    };
    expect(staleOpportunity(o, now)).toBe(true);
    expect(
      staleOpportunity({ ...o, updatedAt: "2026-09-28T00:00:00Z" }, now),
    ).toBe(false);
    expect(staleOpportunity({ ...o, stage: "won" }, now)).toBe(false);
    expect(
      staleOpportunity({ ...o, updatedAt: "0001-01-01T00:00:00Z" }, now),
    ).toBe(true);
  });
  it("calculates calendar date ranges across month and year boundaries", () => {
    expect(dateOffset(-6, "2026-10-02")).toBe("2026-09-26");
    expect(dateOffset(-1, "2026-01-01")).toBe("2025-12-31");
  });
});
