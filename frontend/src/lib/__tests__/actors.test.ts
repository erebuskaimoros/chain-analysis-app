import { describe, expect, it } from "vitest";
import { parseActorAddressLines } from "../actors";
import { makeAnnotation } from "../../test-support/graphFixtures";

describe("parseActorAddressLines", () => {
  it("resolves saved label annotations by name", () => {
    const annotation = makeAnnotation({
      address: "thor1treasury",
      normalized_address: "thor1treasury",
      value: "Treasury Hot Wallet",
    });

    const parsed = parseActorAddressLines("Treasury Hot Wallet", [annotation]);

    expect(parsed.errors).toEqual([]);
    expect(parsed.addresses).toEqual([
      {
        address: "thor1treasury",
        chain_hint: "",
        label: "Treasury Hot Wallet",
      },
    ]);
  });

  it("allows overriding chain and label for a saved annotation", () => {
    const annotation = makeAnnotation({
      address: "0xabcabcabcabcabcabcabcabcabcabcabcabcabca",
      normalized_address: "0xabcabcabcabcabcabcabcabcabcabcabcabcabca",
      value: "Treasury Signer",
    });

    const parsed = parseActorAddressLines("Treasury Signer,BASE,Hot wallet", [annotation]);

    expect(parsed.errors).toEqual([]);
    expect(parsed.addresses).toEqual([
      {
        address: "0xabcabcabcabcabcabcabcabcabcabcabcabcabca",
        chain_hint: "BASE",
        label: "Hot wallet",
      },
    ]);
  });

  it("rejects ambiguous annotation labels", () => {
    const annotations = [
      makeAnnotation({
        address: "thor1first",
        normalized_address: "thor1first",
        value: "Shared Label",
      }),
      makeAnnotation({
        id: 2,
        address: "thor1second",
        normalized_address: "thor1second",
        value: "Shared Label",
      }),
    ];

    const parsed = parseActorAddressLines("Shared Label", annotations);

    expect(parsed.addresses).toEqual([]);
    expect(parsed.errors).toEqual(["Ambiguous annotated address on line 1: Shared Label"]);
  });
});
