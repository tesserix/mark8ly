import { create } from "zustand";

export type CheckoutStep = 1 | 2 | 3 | 4;

export type PaymentProvider = "stripe" | "razorpay" | null;

export interface CheckoutAddress {
  line1: string;
  line2?: string;
  city: string;
  state: string;
  postalCode: string;
  country: string;
  phone?: string;
}

export interface CouponDiscount {
  code: string;
  amount: number;
  type: "fixed" | "percent";
}

export interface GiftCardCredit {
  code: string;
  amount: number;
}

interface CheckoutState {
  step: CheckoutStep;
  email: string;
  customerName: string;
  address: CheckoutAddress | null;
  selectedAddressId: string | null;
  shippingRateId: string | null;
  paymentProvider: PaymentProvider;
  coupon: CouponDiscount | null;
  giftCard: GiftCardCredit | null;
  loyaltyPoints: number;
  saveAddress: boolean;

  setStep: (step: CheckoutStep) => void;
  setContact: (email: string, customerName: string) => void;
  setAddress: (address: CheckoutAddress) => void;
  setSelectedAddressId: (id: string) => void;
  setShippingRate: (rateId: string) => void;
  setPaymentProvider: (provider: PaymentProvider) => void;
  setCoupon: (coupon: CouponDiscount) => void;
  clearCoupon: () => void;
  setGiftCard: (giftCard: GiftCardCredit) => void;
  clearGiftCard: () => void;
  setLoyaltyPoints: (points: number) => void;
  setSaveAddress: (save: boolean) => void;
  reset: () => void;
}

const initialState = {
  step: 1 as CheckoutStep,
  email: "",
  customerName: "",
  address: null,
  selectedAddressId: null,
  shippingRateId: null,
  paymentProvider: null as PaymentProvider,
  coupon: null,
  giftCard: null,
  loyaltyPoints: 0,
  saveAddress: false,
};

export const useCheckoutStore = create<CheckoutState>()((set) => ({
  ...initialState,

  setStep: (step) => set({ step }),

  setContact: (email, customerName) => set({ email, customerName }),

  setAddress: (address) => set({ address, selectedAddressId: null }),

  setSelectedAddressId: (selectedAddressId) =>
    set({ selectedAddressId, address: null }),

  setShippingRate: (shippingRateId) => set({ shippingRateId }),

  setPaymentProvider: (paymentProvider) => set({ paymentProvider }),

  setCoupon: (coupon) => set({ coupon }),

  clearCoupon: () => set({ coupon: null }),

  setGiftCard: (giftCard) => set({ giftCard }),

  clearGiftCard: () => set({ giftCard: null }),

  setLoyaltyPoints: (loyaltyPoints) => set({ loyaltyPoints }),

  setSaveAddress: (saveAddress) => set({ saveAddress }),

  reset: () => set({ ...initialState }),
}));
