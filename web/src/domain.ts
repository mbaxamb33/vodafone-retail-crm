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
  status: z.enum(["active", "archived", "anonymized"]).default("active"),
  tags: z.array(z.string()),
  createdAt: z.string(),
  updatedAt: z.string(),
  lastInteractionAt: z.string().nullish(),
  nextFollowUpDue: z.string().optional(),
});
export const visitSchema = z.object({
  id: z.string(),
  customerId: z.string(),
  employeeId: z.string(),
  at: z.string(),
  reason: z.string(),
  reasonCode: z.string().optional(),
  steps: z.array(z.number()),
  furthestStep: z.number().optional(),
  notes: z.string(),
  notesEditedAt: z.string().optional(),
});
export const followUpSchema = z.object({
  id: z.string(),
  customerId: z.string(),
  employeeId: z.string(),
  type: z.string(),
  due: z.string(),
  status: z.enum(["open", "waiting", "unreachable", "done"]),
  opportunityId: z.string().optional(),
  notes: z.string().optional(),
  completedAt: z.string().optional(),
});
export const opportunitySchema = z.object({
  id: z.string(),
  customerId: z.string(),
  employeeId: z.string(),
  product: z.string(),
  category: z.string().optional(),
  stage: z.string(),
  estimatedValue: z.number().nullish(),
  notes: z.string().optional(),
  createdAt: z.string(),
  stageChangedAt: z.string().optional(),
  closedAt: z.string().optional(),
});
export const workspaceSchema = z.object({
  users: z.array(userSchema),
  customers: z.array(customerSchema),
  followUps: z.array(followUpSchema),
  opportunities: z.array(opportunitySchema),
  today: z.string().optional(),
});
export const pageSchema = <T extends z.ZodTypeAny>(item: T) =>
  z.object({
    items: z.array(item),
    total: z.number(),
    offset: z.number(),
    limit: z.number(),
  });
export const customerPageSchema = pageSchema(customerSchema);
const auditSchema = z.object({
  id: z.string(),
  actorId: z.string(),
  action: z.string(),
  at: z.string(),
  detail: z.string(),
  data: z.record(z.string(), z.unknown()).nullish(),
});
export const profileSchema = z.object({
  customer: customerSchema,
  visits: z.array(visitSchema),
  lastVisit: visitSchema.nullable(),
  total: z.number(),
  followUps: z.array(followUpSchema),
  opportunities: z.array(opportunitySchema),
  ownershipHistory: z.array(auditSchema).default([]),
  audit: z.array(auditSchema),
});
export const meSchema = z.object({
  user: userSchema,
  store: z.object({ id: z.string(), name: z.string(), timezone: z.string() }),
  features: z.array(z.string()),
});
const catalogItem = z.object({ code: z.string(), label: z.string() });
export const catalogSchema = z.object({
  visitReasons: z.array(catalogItem),
  nextActions: z.array(catalogItem),
  productCategories: z.array(catalogItem),
});
const employeeReportSchema = z.object({
  employeeId: z.string(),
  name: z.string(),
  role: z.string(),
  visits: z.number(),
  customersHandled: z.number(),
  newCustomers: z.number(),
  opportunitiesCreated: z.number(),
  offers: z.number(),
  contracts: z.number(),
  portfolioCustomers: z.number(),
  activeOpportunities: z.number(),
  openFollowUps: z.number(),
  overdueFollowUps: z.number(),
});
export const reportSchema = z.object({
  from: z.string(),
  to: z.string(),
  summary: z.object({
    visits: z.number(),
    customersHandled: z.number(),
    newCustomers: z.number(),
    opportunitiesCreated: z.number(),
    offers: z.number(),
    contracts: z.number(),
    lost: z.number(),
    poolCustomers: z.number(),
    activeOpportunities: z.number(),
    followUpsDueToday: z.number(),
    overdueFollowUps: z.number(),
  }),
  funnel: z.array(
    z.object({
      step: z.number(),
      label: z.string(),
      count: z.number(),
      rate: z.number(),
    }),
  ),
  stepIncidence: z.array(
    z.object({ step: z.number(), label: z.string(), count: z.number() }),
  ),
  employees: z.array(employeeReportSchema),
});
export const activitySchema = pageSchema(visitSchema).extend({
  summary: employeeReportSchema,
  customers: z.array(customerSchema).default([]),
});
export const notificationsSchema = z.object({
  items: z.array(
    z.object({
      id: z.string(),
      kind: z.string(),
      customerId: z.string().optional(),
      customerName: z.string().optional(),
      message: z.string(),
      createdAt: z.string(),
      readAt: z.string().optional(),
    }),
  ),
  unread: z.number(),
});
export type FollowUp = z.infer<typeof followUpSchema>;
export type Opportunity = z.infer<typeof opportunitySchema>;
export type Report = z.infer<typeof reportSchema>;
export type Catalog = z.infer<typeof catalogSchema>;
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
  const at = o.stageChangedAt ?? o.createdAt;
  return activeOpportunity(o) && now - Date.parse(at) >= 7 * 86400000;
}
export const lastInteraction = (c: Customer) =>
  c.lastInteractionAt ?? c.createdAt;
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
