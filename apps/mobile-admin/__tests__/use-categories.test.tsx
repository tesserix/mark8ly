import { createCategoriesApi } from "@repo/mobile-shared/api/categories";

describe("createCategoriesApi", () => {
  it("GETs /categories and validates with the {data} schema", async () => {
    const get = jest.fn().mockResolvedValue({ data: [] });
    const api = createCategoriesApi({ get } as never);
    await api.list();
    expect(get).toHaveBeenCalledTimes(1);
    const [path, params, schema] = get.mock.calls[0]!;
    expect(path).toBe("/categories");
    expect(params).toBeUndefined();
    // The schema must be passed — an unvalidated response is exactly how the
    // {items} fiction hid 161 products for two months.
    expect(schema).toBeDefined();
  });
});
