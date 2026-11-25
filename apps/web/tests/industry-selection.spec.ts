import {expect, test} from "@playwright/test";
import {buildRequirements, type Offering} from "../src/lib/industry-workflows";

const offering: Offering = {
  id: "01010101-0101-4101-a101-010101010101", name: "Synthetic service", revision: 1,
  timezone: "Europe/Moscow", min_minutes: 60, max_minutes: 60,
};

function slotForDevice(iso: string) {
  const date = new Date(iso);
  const two = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${two(date.getMonth() + 1)}-${two(date.getDate())}T${two(date.getHours())}:${two(date.getMinutes())}`;
}

function requirements(start: string, timezone: string) {
  const form = new FormData();
  for (const [key, value] of Object.entries({
    make: "Synthetic", model: "Example", engine: "Synthetic", year: "2025",
    slot: slotForDevice(start), duration: "60",
  })) form.set(key, value);
  return buildRequirements("auto-service", form, {...offering, timezone});
}

test("sends the selected instant with the service timezone offset", () => {
  expect(requirements("2027-02-15T21:30:00Z", "Europe/Moscow")).toMatchObject({
    selection: {timezone: "Europe/Moscow", starts_at: "2027-02-16T00:30:00+03:00", ends_at: "2027-02-16T01:30:00+03:00"},
  });
  expect(requirements("2027-02-15T21:30:00Z", "UTC")).toMatchObject({
    selection: {starts_at: "2027-02-15T21:30:00Z", ends_at: "2027-02-15T22:30:00Z"},
  });
});

test("uses each endpoint's offset when the service crosses a DST change", () => {
  expect(requirements("2027-03-28T00:30:00Z", "Europe/Berlin")).toMatchObject({
    selection: {starts_at: "2027-03-28T01:30:00+01:00", ends_at: "2027-03-28T03:30:00+02:00"},
  });
});
