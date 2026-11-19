function makeAcc(start) {
	let total = start;
	return function (delta) {
		total += delta;
		return total;
	};
}

function main() {
	let sum = 0;
	for (let i = 0; i < 50000; i++) {
		const acc = makeAcc(i);
		acc(3);
		acc(7);
		sum += acc(-2);
	}

	const big = makeAcc(0);
	for (let j = 0; j < 1000000; j++) {
		big(j % 7);
	}
	sum += big(0);
	return sum;
}

console.log(main());
