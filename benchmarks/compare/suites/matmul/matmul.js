function matmul(n) {
	const a = [];
	const b = [];
	for (let i = 0; i < n; i++) {
		const ra = [];
		const rb = [];
		for (let j = 0; j < n; j++) {
			ra.push((i * 3 + j * 7) % 17);
			rb.push((i * 5 + j * 11) % 19);
		}
		a.push(ra);
		b.push(rb);
	}

	let total = 0;
	for (let i = 0; i < n; i++) {
		const ai = a[i];
		for (let j = 0; j < n; j++) {
			let acc = 0;
			for (let k = 0; k < n; k++) {
				acc += ai[k] * b[k][j];
			}
			total += acc;
		}
	}
	return total;
}

console.log(matmul(120));
