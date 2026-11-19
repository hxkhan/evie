local function sieve(n)
	local flags = {}
	for i = 1, n do
		flags[i] = 0
	end

	local i = 2
	while i * i < n do
		if flags[i + 1] == 0 then
			local j = i * i
			while j < n do
				flags[j + 1] = 1
				j = j + i
			end
		end
		i = i + 1
	end

	local count = 0
	for k = 2, n - 1 do
		if flags[k + 1] == 0 then
			count = count + 1
		end
	end
	return count
end

print(sieve(1000000))
