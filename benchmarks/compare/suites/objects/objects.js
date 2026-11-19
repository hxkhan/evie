function main() {
	const keys = [];
	for (let i = 0; i < 500; i++) {
		keys.push("k" + i);
	}

	let total = 0;
	for (let r = 0; r < 400; r++) {
		const m = {};
		for (let j = 0; j < 500; j++) {
			m[keys[j]] = j + r;
		}
		for (let j = 0; j < 500; j++) {
			const k = keys[j];
			m[k] = m[k] + (j % 5);
		}
		for (let j = 0; j < 500; j++) {
			total += m[keys[j]];
		}
	}
	return total;
}

console.log(main());
