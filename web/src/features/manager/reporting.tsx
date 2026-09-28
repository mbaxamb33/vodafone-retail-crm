import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../api";
import { activitySchema, dateOffset, today, reportSchema } from "../../domain";

const range = (start: string, end: string) =>
  start ? "from=" + start + "&to=" + end : "";

export function useStoreReport() {
  const [period, setPeriod] = useState("30");
  const [from, setFrom] = useState(dateOffset(-29));
  const [to, setTo] = useState(today());
  const start =
    period === "all"
      ? ""
      : period === "custom"
        ? from
        : dateOffset(-(Number(period) - 1));
  const end = period === "all" ? "" : period === "custom" ? to : today();
  const valid = period !== "custom" || Boolean(from && to && from <= to);
  const query = useQuery({
    queryKey: ["manager", start, end],
    queryFn: async () =>
      reportSchema.parse(await api("manager/dashboard?" + range(start, end))),
    enabled: valid,
  });
  const controls = (
    <div className="report-controls">
      <label htmlFor="report-period">Perioada activității</label>
      <select
        id="report-period"
        value={period}
        onChange={(e) => setPeriod(e.target.value)}
      >
        <option value="1">Astăzi</option>
        <option value="7">Ultimele 7 zile</option>
        <option value="30">Ultimele 30 de zile</option>
        <option value="all">Toată perioada</option>
        <option value="custom">Alege un interval</option>
      </select>
      {period === "custom" && (
        <div className="date-range">
          <label>
            De la
            <input
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </label>
          <label>
            Până la
            <input
              type="date"
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
          </label>
        </div>
      )}
      {!valid && (
        <p role="alert" className="error">
          Alege un interval valid.
        </p>
      )}
    </div>
  );
  return { query, controls, valid, start, end };
}

// useEmployeeActivity pages through one colleague's visits in the selected period.
export function useEmployeeActivity(
  employeeId: string | undefined,
  start: string,
  end: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["manager", "activity", employeeId, start, end],
    queryFn: async () =>
      activitySchema.parse(
        await api(
          `manager/employees/${employeeId}/activity?limit=100&${range(start, end)}`,
        ),
      ),
    enabled: enabled && !!employeeId,
  });
}
