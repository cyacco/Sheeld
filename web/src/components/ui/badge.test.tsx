import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Badge } from "./badge";

// A minimal render test to prove the jsdom + Testing Library harness works.
describe("Badge", () => {
  it("renders its children", () => {
    render(<Badge>shadow</Badge>);
    expect(screen.getByText("shadow")).toBeInTheDocument();
  });

  it("reflects the variant via data attribute", () => {
    render(<Badge variant="destructive">fail</Badge>);
    expect(screen.getByText("fail")).toHaveAttribute("data-variant", "destructive");
  });
});
