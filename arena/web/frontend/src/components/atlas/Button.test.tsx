import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { Button } from "./Button";

describe("Button", () => {
  it("renders a button with the child label and fires onClick for primary variant", () => {
    const onClick = vi.fn();
    render(
      <Button variant="primary" onClick={onClick}>
        Launch
      </Button>,
    );
    const btn = screen.getByRole("button", { name: "Launch" });
    expect(btn).toBeInTheDocument();
    fireEvent.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
  });
});
