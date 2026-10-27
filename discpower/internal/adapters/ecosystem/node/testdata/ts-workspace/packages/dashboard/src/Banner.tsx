import { discount } from "../../rates/src/rates";

export function Banner({ total, percent }: { total: number; percent: number }) {
  return <p className="banner">Now {discount(total, percent)}</p>;
}
