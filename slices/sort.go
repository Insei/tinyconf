package slices

// insertionSortCmpFunc sorts data[a:b] using insertion sort.
func insertionSortCmpFunc[E any](data []E, a, b int, cmp func(a, b E) int) {
	for i := a + 1; i < b; i++ {
		for j := i; j > a && (cmp(data[j], data[j-1]) < 0); j-- {
			data[j], data[j-1] = data[j-1], data[j]
		}
	}
}

func swapRangeCmpFunc[E any](data []E, a, b, n int, cmp func(a, b E) int) {
	for i := 0; i < n; i++ {
		data[a+i], data[b+i] = data[b+i], data[a+i]
	}
}

// rotateCmpFunc rotates two consecutive blocks u = data[a:m] and v = data[m:b] in data:
// Data of the form 'x u v y' is changed to 'x v u y'.
// rotate performs at most b-a many calls to data.Swap,
// and it assumes non-degenerate arguments: a < m && m < b.
func rotateCmpFunc[E any](data []E, a, m, b int, cmp func(a, b E) int) {
	i := m - a
	j := b - m

	for i != j {
		if i > j {
			swapRangeCmpFunc(data, m-i, m, j, cmp)
			i -= j
		} else {
			swapRangeCmpFunc(data, m-i, m+j-i, i, cmp)
			j -= i
		}
	}
	// i == j
	swapRangeCmpFunc(data, m-i, m, i, cmp)
}

// symMergeCmpFunc merges the two sorted subsequences data[a:m] and data[m:b] using
// the SymMerge algorithm from Pok-Son Kim and Arne Kutzner, "Stable Minimum
// Storage Merging by Symmetric Comparisons", in Susanne Albers and Tomasz
// Radzik, editors, Algorithms - ESA 2004, volume 3221 of Lecture Notes in
// Computer Science, pages 714-723. Springer, 2004.
//
// Let M = m-a and N = b-n. Wolog M < N.
// The recursion depth is bound by ceil(log(N+M)).
// The algorithm needs O(M*log(N/M + 1)) calls to data.Less.
// The algorithm needs O((M+N)*log(M)) calls to data.Swap.
//
// The paper gives O((M+N)*log(M)) as the number of assignments assuming a
// rotation algorithm which uses O(M+N+gcd(M+N)) assignments. The argumentation
// in the paper carries through for Swap operations, especially as the block
// swapping rotate uses only O(M+N) Swaps.
//
// symMerge assumes non-degenerate arguments: a < m && m < b.
// Having the caller check this condition eliminates many leaf recursion calls,
// which improves performance.
func symMergeCmpFunc[E any](data []E, a, m, b int, cmp func(a, b E) int) {
	// Avoid unnecessary recursions of symMerge
	// by direct insertion of data[a] into data[m:b]
	// if data[a:m] only contains one element.
	if m-a == 1 {
		// Use binary search to find the lowest index i
		// such that data[i] >= data[a] for m <= i < b.
		// Exit the search loop with i == b in case no such index exists.
		i := m
		j := b
		for i < j {
			h := int(uint(i+j) >> 1)
			if cmp(data[h], data[a]) < 0 {
				i = h + 1
			} else {
				j = h
			}
		}
		// Swap values until data[a] reaches the position before i.
		for k := a; k < i-1; k++ {
			data[k], data[k+1] = data[k+1], data[k]
		}
		return
	}

	// Avoid unnecessary recursions of symMerge
	// by direct insertion of data[m] into data[a:m]
	// if data[m:b] only contains one element.
	if b-m == 1 {
		// Use binary search to find the lowest index i
		// such that data[i] > data[m] for a <= i < m.
		// Exit the search loop with i == m in case no such index exists.
		i := a
		j := m
		for i < j {
			h := int(uint(i+j) >> 1)
			if !(cmp(data[m], data[h]) < 0) {
				i = h + 1
			} else {
				j = h
			}
		}
		// Swap values until data[m] reaches the position i.
		for k := m; k > i; k-- {
			data[k], data[k-1] = data[k-1], data[k]
		}
		return
	}

	mid := int(uint(a+b) >> 1)
	n := mid + m
	var start, r int
	if m > mid {
		start = n - b
		r = mid
	} else {
		start = a
		r = m
	}
	p := n - 1

	for start < r {
		c := int(uint(start+r) >> 1)
		if !(cmp(data[p-c], data[c]) < 0) {
			start = c + 1
		} else {
			r = c
		}
	}

	end := n - start
	if start < m && m < end {
		rotateCmpFunc(data, start, m, end, cmp)
	}
	if a < start && start < mid {
		symMergeCmpFunc(data, a, start, mid, cmp)
	}
	if mid < end && end < b {
		symMergeCmpFunc(data, mid, end, b, cmp)
	}
}

func stableCmpFunc[E any](data []E, n int, cmp func(a, b E) int) {
	blockSize := 20 // must be > 0
	a, b := 0, blockSize
	for b <= n {
		insertionSortCmpFunc(data, a, b, cmp)
		a = b
		b += blockSize
	}
	insertionSortCmpFunc(data, a, n, cmp)

	for blockSize < n {
		a, b = 0, 2*blockSize
		for b <= n {
			symMergeCmpFunc(data, a, a+blockSize, b, cmp)
			a = b
			b += 2 * blockSize
		}
		if m := a + blockSize; m < n {
			symMergeCmpFunc(data, a, m, n, cmp)
		}
		blockSize *= 2
	}
}

// SortStableFunc sorts the slice x while keeping the original order of equal
// elements, using cmp to compare elements in the same way as [SortFunc].
func SortStableFunc[S ~[]E, E any](x S, cmp func(a, b E) int) {
	stableCmpFunc(x, len(x), cmp)
}
