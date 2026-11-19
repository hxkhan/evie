function mandelbrot(size, maxIter) {
	let total = 0;
	for (let py = 0; py < size; py++) {
		const y0 = (py / size) * 2.0 - 1.0;
		for (let px = 0; px < size; px++) {
			const x0 = (px / size) * 3.5 - 2.5;
			let x = 0.0;
			let y = 0.0;
			let iter = 0;
			while (iter < maxIter) {
				const xx = x * x;
				const yy = y * y;
				if (xx + yy > 4.0) break;
				y = 2.0 * x * y + y0;
				x = xx - yy + x0;
				iter++;
			}
			total += iter;
		}
	}
	return total;
}

console.log(mandelbrot(400, 50));
