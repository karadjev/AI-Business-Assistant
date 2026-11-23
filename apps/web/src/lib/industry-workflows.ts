export const scenarios = [
  {id: "auto-service", label: "Автосервис · проверить запись"},
  {id: "construction", label: "Стройматериалы · рассчитать предложение"},
  {id: "auto-parts", label: "Автозапчасти · проверить совместимость"},
  {id: "real-estate", label: "Недвижимость · подобрать объекты"},
] as const;

export type Scenario = typeof scenarios[number]["id"];
export type Offering = {
  id: string; name: string; revision: number; timezone: string; min_minutes: number; max_minutes: number;
  part_rule?: unknown;
};
export type IndustryTask = {id: string; status: string; result: unknown};

export const uuidPattern = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/i;

export function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown> : undefined;
}

export function readID(value: unknown): string | undefined {
  const id = record(value)?.id;
  return typeof id === "string" && uuidPattern.test(id) ? id : undefined;
}

export function parseOfferings(value: unknown): Offering[] {
  if (!Array.isArray(value)) throw new Error("Не удалось прочитать список услуг.");
  return value.map((item: unknown) => {
    const row = record(item);
    const id = readID(item);
    if (!row || !id || typeof row.name !== "string" || typeof row.timezone !== "string"
        || typeof row.revision !== "number" || !Number.isSafeInteger(row.revision) || row.revision < 1
        || typeof row.min_minutes !== "number" || typeof row.max_minutes !== "number"
        || !Number.isSafeInteger(row.min_minutes) || !Number.isSafeInteger(row.max_minutes)
        || row.min_minutes < 1 || row.max_minutes < row.min_minutes) {
      throw new Error("Не удалось прочитать параметры услуги.");
    }
    return {id, name: row.name, revision: row.revision, timezone: row.timezone,
      min_minutes: row.min_minutes, max_minutes: row.max_minutes, part_rule: row.part_rule};
  });
}

function text(form: FormData, key: string) {
  const value = form.get(key);
  return typeof value === "string" ? value.trim() : "";
}

function number(form: FormData, key: string, min: number, max: number) {
  const value = text(form, key);
  const parsed = Number(value);
  if (!/^\d+$/.test(value) || !Number.isSafeInteger(parsed) || parsed < min || parsed > max) {
    throw new Error("Проверьте числовые поля.");
  }
  return parsed;
}

function measure(form: FormData, key: string, allowZero = false) {
  const value = text(form, key).replace(",", ".");
  if (!/^\d+(\.\d{1,9})?$/.test(value) || !Number.isFinite(Number(value)) || (!allowZero && Number(value) <= 0)) {
    throw new Error("Укажите размеры и запас десятичными числами.");
  }
  return value;
}

function serviceSelection(form: FormData, offering: Offering) {
  const slot = text(form, "slot");
  const start = new Date(slot);
  const [year, month, day, hour, minute] = slot.split(/[-T:]/).map(Number);
  if (!Number.isFinite(start.getTime()) || start.getFullYear() !== year || start.getMonth() + 1 !== month
      || start.getDate() !== day || start.getHours() !== hour || start.getMinutes() !== minute) {
    throw new Error("Выберите существующее местное время.");
  }
  const duration = number(form, "duration", offering.min_minutes, offering.max_minutes);
  return {offering_id: offering.id, offering_revision: offering.revision, timezone: offering.timezone,
    starts_at: start.toISOString(), ends_at: new Date(start.getTime() + duration * 60_000).toISOString(), units: 1};
}

export function buildRequirements(scenario: Scenario, form: FormData, offering?: Offering) {
  if (scenario === "construction") {
    const dimension = (key: string) => ({value: measure(form, key), unit: "m"});
    const operation = text(form, "calculation_kind") === "blocks" ? "blocks" : "tiles";
    return {sku: text(form, "sku"), calculation: {
      operation, length: dimension("length"), width: dimension("width"),
      item_length: dimension("item_length"), item_width: dimension("item_width"),
      waste_percent: measure(form, "waste_percent", true),
    }};
  }
  if (scenario === "real-estate") {
    const budget = text(form, "budget")
      ? {max_price_amount: number(form, "budget", 0, Math.floor(Number.MAX_SAFE_INTEGER / 100)) * 100, currency_code: "RUB"}
      : {};
    return {city: text(form, "city"), property_type: text(form, "property_type"),
      transaction_type: text(form, "transaction_type"), wishes: text(form, "wishes"), ...budget};
  }
  const vehicle = {make: text(form, "make"), model: text(form, "model"), engine: text(form, "engine"),
    year: number(form, "year", 1886, 2100)};
  const oem = text(form, "oem");
  if (scenario === "auto-parts") {
    return {vehicle, oem, quantity: number(form, "quantity", 1, 10)};
  }
  if (!offering) throw new Error("Выберите услугу из результатов поиска.");
  const part = oem ? {part: {vehicle, oem, quantity: number(form, "quantity", 1, 10)}} : {};
  return {vehicle, selection: serviceSelection(form, offering), notes: text(form, "notes"), ...part};
}
