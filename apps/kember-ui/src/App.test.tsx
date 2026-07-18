import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { App } from "./App";

describe("App", () => {
  it("renders Kember navigation", () => {
    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "Worker pools" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Task runs" })).toBeTruthy();
  });
});
