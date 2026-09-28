import { z } from "zod";
export const userSchema = z.object({
  id: z.string(),
  name: z.string(),
  role: z.enum(["employee", "manager"]),
  storeId: z.string(),
});
export const customerSchema = z.object({
  id: z.string(),
  name: z.string(),
  phone: z.string(),
  storeId: z.string(),
  ownerId: z.string(),
  ownership: z.enum(["owned", "pool", "unassigned"]),
  tags: z.array(z.string()),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export const visitSchema = z.object({
  id: z.string(),
  customerId: z.string(),
  employeeId: z.string(),
  at: z.string(),
  reason: z.string(),
  steps: z.array(z.number()),
  notes: z.string(),
});
export const followUpSchema = z.object({
  id: z.string(),
  customerId: z.string(),
  employeeId: z.string(),
  type: z.string(),
  due: z.string(),
  status: z.enum(["open", "waiting", "unreachable", "done"]),
  opportunityId: z.string().optional(),
  completedAt: z.string().optional(),
});
export const opportunitySchema = z.object({
  id: z.string(),
  customerId: z.string(),
  employeeId: z.string(),
  product: z.string(),
  stage: z.string(),
  createdAt: z.string(),
  updatedAt: z.string().optional(),
});
export const workspaceSchema = z.object({
  users: z.array(userSchema),
  customers: z.array(customerSchema),
  followUps: z.array(followUpSchema),
  opportunities: z.array(opportunitySchema),
});
export const profileSchema = z.object({
  customer: customerSchema,
  visits: z.array(visitSchema),
  lastVisit: visitSchema.nullable(),
  total: z.number(),
  followUps: z.array(followUpSchema),
  opportunities: z.array(opportunitySchema),
  audit: z.array(
    z.object({
      id: z.string(),
      actorId: z.string(),
      action: z.string(),
      at: z.string(),
      detail: z.string(),
    }),
  ),
});
export const reportSchema = z.object({
  visits: z.array(visitSchema),
  newCustomers: z.array(customerSchema),
  events: z.array(
    z.object({
      id: z.string(),
      customerId: z.string(),
      employeeId: z.string(),
      stage: z.string(),
      at: z.string(),
    }),
  ),
  from: z.string(),
  to: z.string(),
});
export type FollowUp = z.infer<typeof followUpSchema>;
export type Opportunity = z.infer<typeof opportunitySchema>;
export type Report = z.infer<typeof reportSchema>;
export const followUpStatuses: Record<FollowUp["status"], string> = {
  open: "Programat",
  waiting: "Așteptăm clientul",
  unreachable: "Nu a răspuns",
  done: "Finalizat",
};
export const activeOpportunity = (o: Opportunity) =>
  !["won", "lost"].includes(o.stage);
export function nextOpportunityAction(o: Opportunity, followUps: FollowUp[]) {
  return followUps
    .filter((f) => f.opportunityId === o.id && f.status !== "done")
    .sort((a, b) => a.due.localeCompare(b.due))[0];
}
export function staleOpportunity(o: Opportunity, now = Date.now()) {
  const at =
    o.updatedAt && o.updatedAt !== "0001-01-01T00:00:00Z"
      ? o.updatedAt
      : o.createdAt;
  return activeOpportunity(o) && now - Date.parse(at) >= 7 * 86400000;
}
export function dateOffset(days: number, base = today()) {
  const d = new Date(base + "T12:00:00Z");
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}
export type User = z.infer<typeof userSchema>;
export type Customer = z.infer<typeof customerSchema>;
export type Workspace = z.infer<typeof workspaceSchema>;
export const steps = [
  "Welcome",
  "Rezolvarea solicitării",
  "Small talk",
  "Atenție și permisiune",
  "Verificare",
  "Prezentare",
  "Ofertă",
  "Contractare",
];
export const stages: Record<string, string> = {
  identified: "Identificată",
  qualified: "Calificată",
  verification: "Verificare",
  presentation: "Prezentare",
  offer: "Ofertă",
  waiting: "În așteptare",
  won: "Câștigată",
  lost: "Pierdută",
  paused: "În pauză",
};
export function normalizePhone(p: string) {
  let v = p.replace(/[\s()-]/g, "");
  if (v.startsWith("00")) v = "+" + v.slice(2);
  if (/^0\d{9}$/.test(v)) v = "+40" + v.slice(1);
  return v;
}
export function matches(c: Customer, q: string) {
  return (
    c.name.toLocaleLowerCase("ro").includes(q.toLocaleLowerCase("ro")) ||
    c.phone.includes(normalizePhone(q))
  );
}
export const today = () =>
  new Intl.DateTimeFormat("sv-SE", { timeZone: "Europe/Bucharest" }).format(
    new Date(),
  );
export const date = (s: string) =>
  new Intl.DateTimeFormat("ro-RO", {
    day: "numeric",
    month: "short",
    timeZone: "Europe/Bucharest",
  }).format(new Date(s.length === 10 ? s + "T12:00:00Z" : s));
export const initials = (s: string) =>
  s
    .split(" ")
    .map((x) => x[0])
    .slice(0, 2)
    .join("");
