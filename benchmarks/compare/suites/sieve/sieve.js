function sieve(n) {
	const flags = [];
	for (let i = 0; i < n; i++) {
		flags.push(0);
	}

	for (let i = 2; i * i < n; i++) {
		if (flags[i] === 0) {
			for (let j = i * i; j < n; j += i) {
				flags[j] = 1;
			}
		}
	}

	let count = 0;
	for (let i = 2; i < n; i++) {
		if (flags[i] === 0) {
			count++;
		}
	}
	return count;
}

console.log(sieve(1000000));
