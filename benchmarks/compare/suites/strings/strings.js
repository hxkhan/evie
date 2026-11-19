function main() {
	const base = "alpha,beta,gamma,delta,epsilon";
	let total = 0;
	for (let i = 0; i < 40000; i++) {
		const s = base + "," + i;
		const parts = s.split(",");
		const joined = parts.join("-");
		const replaced = joined.replaceAll("beta", "BETA");
		if (replaced.includes("gamma")) {
			total += replaced.length;
		}
	}
	return total;
}

console.log(main());
