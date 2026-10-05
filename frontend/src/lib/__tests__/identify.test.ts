import { describe, expect, it } from "vitest";
import { encodeSeed, guessAddressChain, identifyInput } from "../identify";

const THOR = `thor1${"q".repeat(38)}`;

describe("identifyInput", () => {
  it("recognises transaction hashes with or without 0x", () => {
    const hash = "A732E09AAB76A571D768C2B5B4E2F0E1E5B1A9C3D4E5F60718293A4B5C6D7E8F";
    expect(identifyInput(hash)).toEqual({ kind: "tx", value: hash });
    expect(identifyInput(`0x${hash.toLowerCase()}`).kind).toBe("tx");
  });

  it("names the chain of addresses it can tell apart", () => {
    expect(identifyInput(THOR)).toMatchObject({ kind: "address", chain: "THOR", chainCertain: true });
    expect(identifyInput(`maya1${"q".repeat(38)}`)).toMatchObject({ chain: "MAYA", chainCertain: true });
    expect(identifyInput("bc1qvqja3gaa73ku037yutpt6h8l788rva70cqj68f")).toMatchObject({ chain: "BTC", chainCertain: true });
    expect(identifyInput("qz7262r7d0tvv4kfqfv8kgaykflm4t5xcu6c7vskzw")).toMatchObject({ chain: "BCH", chainCertain: true });
    expect(identifyInput("DH5yaieqoZN36fDVciNyRueRGvGLR3mr7L")).toMatchObject({ chain: "DOGE", chainCertain: true });
    expect(identifyInput("TJ8LE5jBg4D4nGyk6RjDDXAvmHjSny1dHT")).toMatchObject({ chain: "TRON", chainCertain: true });
    expect(identifyInput("7EcDhSYGxXyscszYEp35KHN8vvw3svAuLKTzXwCFLtV")).toMatchObject({ chain: "SOL", chainCertain: true });
  });

  it("treats EVM addresses as a guess because every EVM chain shares the format", () => {
    expect(identifyInput("0xf7bc92103f23ef312658cd9b81dc2713f7b396c3")).toMatchObject({
      kind: "address",
      chain: "ETH",
      chainCertain: false,
    });
  });

  it("reads CHAIN|address as an explicit chain", () => {
    expect(identifyInput("bsc|0xf7bc92103f23ef312658cd9b81dc2713f7b396c3")).toEqual({
      kind: "address",
      value: "0xf7bc92103f23ef312658cd9b81dc2713f7b396c3",
      chain: "BSC",
      chainCertain: true,
    });
  });

  it("falls back to text for names and partial strings", () => {
    expect(identifyInput("TC Treasury").kind).toBe("text");
    expect(identifyInput("thor1abc").kind).toBe("text");
    expect(identifyInput("   ").kind).toBe("empty");
  });
});

describe("guessAddressChain and encodeSeed", () => {
  it("returns null for strings that are not addresses", () => {
    expect(guessAddressChain("Treasury Hot Wallet")).toBeNull();
  });

  it("writes seeds the way the APIs read them", () => {
    expect(encodeSeed(THOR, "thor")).toBe(`THOR|${THOR}`);
    expect(encodeSeed(THOR)).toBe(THOR);
  });
});
