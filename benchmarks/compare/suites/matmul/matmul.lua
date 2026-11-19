local function matmul(n)
	local a = {}
	local b = {}
	for i = 0, n - 1 do
		local ra = {}
		local rb = {}
		for j = 0, n - 1 do
			ra[j + 1] = (i * 3 + j * 7) % 17
			rb[j + 1] = (i * 5 + j * 11) % 19
		end
		a[i + 1] = ra
		b[i + 1] = rb
	end

	local total = 0
	for i = 1, n do
		local ai = a[i]
		for j = 1, n do
			local acc = 0
			for k = 1, n do
				acc = acc + ai[k] * b[k][j]
			end
			total = total + acc
		end
	end
	return total
end

print(matmul(120))
