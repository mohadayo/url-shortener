import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { APIClient } from "./api.js";

describe("APIClient", () => {
  it("should construct with default base URL", () => {
    const client = new APIClient();
    assert.ok(client);
  });

  it("should construct with custom base URL", () => {
    const client = new APIClient("http://custom:9090");
    assert.ok(client);
  });

  it("should strip trailing slashes from base URL", () => {
    const client = new APIClient("http://localhost:8080///");
    assert.ok(client);
  });
});
