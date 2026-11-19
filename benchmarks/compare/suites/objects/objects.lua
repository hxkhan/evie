local function main()
	local keys = {}
	for i = 0, 499 do
		keys[i + 1] = "k" .. i
	end

	local total = 0
	for r = 0, 399 do
		local m = {}
		for j = 0, 499 do
			m[keys[j + 1]] = j + r
		end
		for j = 0, 499 do
			local k = keys[j + 1]
			m[k] = m[k] + (j % 5)
		end
		for j = 0, 499 do
			total = total + m[keys[j + 1]]
		end
	end
	return total
end

print(main())
