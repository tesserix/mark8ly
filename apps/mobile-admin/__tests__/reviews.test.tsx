jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { render } from "@testing-library/react-native";
import { nextReviewPage } from "@/lib/hooks/use-reviews";
import { ReviewStars } from "@/components/reviews/ReviewStars";
import { ReviewStatusBadge } from "@/components/reviews/ReviewStatusBadge";
import type { ReviewListResponse } from "@repo/mobile-shared/api/schemas/reviews";

function pageOf(page: number, total_pages: number): ReviewListResponse {
  return { data: [], meta: { page, page_size: 50, total: total_pages * 50, total_pages } };
}

describe("nextReviewPage", () => {
  it("advances while pages remain, stops on the last page", () => {
    expect(nextReviewPage(pageOf(1, 3))).toBe(2);
    expect(nextReviewPage(pageOf(3, 3))).toBeUndefined();
    expect(nextReviewPage(pageOf(1, 1))).toBeUndefined();
  });
});

describe("ReviewStars", () => {
  it("rounds the rating to whole filled stars", () => {
    const { getByLabelText } = render(<ReviewStars rating={3.4} />);
    expect(getByLabelText("3 out of 5 stars")).toBeTruthy();
  });

  it("clamps out-of-range ratings to 0..5", () => {
    const hi = render(<ReviewStars rating={9} />);
    expect(hi.getByLabelText("5 out of 5 stars")).toBeTruthy();
    const lo = render(<ReviewStars rating={-2} />);
    expect(lo.getByLabelText("0 out of 5 stars")).toBeTruthy();
  });
});

describe("ReviewStatusBadge", () => {
  it("labels each status", () => {
    expect(render(<ReviewStatusBadge status="pending" />).getByLabelText("Status: Pending")).toBeTruthy();
    expect(render(<ReviewStatusBadge status="approved" />).getByLabelText("Status: Approved")).toBeTruthy();
    expect(render(<ReviewStatusBadge status="rejected" />).getByLabelText("Status: Rejected")).toBeTruthy();
  });
});
