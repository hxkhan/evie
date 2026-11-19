class Adder {
	constructor(base, step) {
		this.base = base;
		this.step = step;
	}

	adjust(x) {
		this.base = (this.base + this.step) % 1000;
		return this.base + x;
	}

	compute(x) {
		return this.adjust(x) + this.step;
	}
}

class Scaler {
	constructor(base, factor) {
		this.base = base;
		this.factor = factor;
	}

	adjust(x) {
		this.base = (this.base + x) % 1000;
		return this.base * this.factor;
	}

	compute(x) {
		return this.adjust(x) - this.factor;
	}
}

function main() {
	const items = [];
	for (let i = 0; i < 200; i++) {
		if (i % 2 === 0) {
			items.push(new Adder(i, (i % 7) + 1));
		} else {
			items.push(new Scaler(i, (i % 5) + 1));
		}
	}

	let total = 0;
	for (let r = 0; r < 2000; r++) {
		const x = r % 13;
		for (let j = 0; j < items.length; j++) {
			total += items[j].compute(x);
		}
	}
	return total;
}

console.log(main());
