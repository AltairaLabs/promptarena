import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";

describe("RTL harness", () => {
  it("renders a DOM node", () => {
    render(<button>hello</button>);
    expect(screen.getByRole("button", { name: "hello" })).toBeInTheDocument();
  });
});
