package flashduty

import "testing"

func TestPhoneIsSame(t *testing.T) {
	tests := []struct {
		name   string
		phone1 string
		phone2 string
		same   bool
	}{
		{
			name:   "blank",
			phone1: "",
			phone2: "",
			same:   true,
		},
		{
			name:   "China +86 prefix",
			phone1: "+8613812345678",
			phone2: "13812345678",
			same:   true,
		},
		{
			name:   "China +86 with spaces and hyphens",
			phone1: "+86 138-1234-5678",
			phone2: "13812345678",
			same:   true,
		},
		{
			name:   "USA +1 prefix",
			phone1: "+1 234-567-8900",
			phone2: "2345678900",
			same:   true,
		},
		{
			name:   "UK +44 prefix",
			phone1: "+442078765432",
			phone2: "2078765432",
			same:   true,
		},
		{
			name:   "India +91 prefix",
			phone1: "+919876543210",
			phone2: "9876543210",
			same:   true,
		},
		{
			name:   "Germany +49 prefix",
			phone1: "+4915123456789",
			phone2: "15123456789",
			same:   true,
		},
		{
			name:   "Different numbers",
			phone1: "+8613812345678",
			phone2: "13812345679",
			same:   false,
		},
	}

	for _, tt := range tests {
		if got := PhoneIsSame(tt.phone1, tt.phone2); got != tt.same {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.same, got)
		}
	}
}
