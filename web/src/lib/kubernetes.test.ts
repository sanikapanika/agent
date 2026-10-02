import { describe, expect, it } from "vitest";
import { describeRef, labelCommands } from "./kubernetes";

describe("labelCommands", () => {
  it("groups resources by namespace and kind", () => {
    expect(
      labelCommands([
        { kind: "service", namespace: "shop", name: "checkout" },
        { kind: "service", namespace: "shop", name: "cart" },
        { kind: "cronjob", namespace: "shop", name: "backup" },
        { kind: "service", namespace: "web", name: "site" },
      ]),
    ).toEqual([
      "kubectl -n shop label service checkout cart upti.my/monitor=true --overwrite",
      "kubectl -n shop label cronjob backup upti.my/monitor=true --overwrite",
      "kubectl -n web label service site upti.my/monitor=true --overwrite",
    ]);
    expect(labelCommands([{ kind: "deployment", namespace: "shop", name: "api" }], true)).toEqual([
      "kubectl -n shop label deployment api upti.my/monitor-",
    ]);
  });
});

describe("describeRef", () => {
  it("names the object a monitor came from", () => {
    expect(describeRef("ingress/shop/shop#shop.example.com")).toBe("ingress shop/shop");
  });
});
