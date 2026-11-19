local function mandelbrot(size, maxIter)
	local total = 0
	for py = 0, size - 1 do
		local y0 = (py / size) * 2.0 - 1.0
		for px = 0, size - 1 do
			local x0 = (px / size) * 3.5 - 2.5
			local x = 0.0
			local y = 0.0
			local iter = 0
			while iter < maxIter do
				local xx = x * x
				local yy = y * y
				if xx + yy > 4.0 then
					break
				end
				y = 2.0 * x * y + y0
				x = xx - yy + x0
				iter = iter + 1
			end
			total = total + iter
		end
	end
	return total
end

print(mandelbrot(400, 50))
