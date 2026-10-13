package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ---------- Models ----------

type MenuItem struct {
	Code  string
	Name  string
	Price int
}

type Restaurant struct {
	ID   int
	Name string
	Loc  string
	Menu []MenuItem
}

type Cart struct {
	Restaurant *Restaurant
	Items      []MenuItem
}

func (c *Cart) AddToCart(it MenuItem) { c.Items = append(c.Items, it) }
func (c *Cart) IsEmpty() bool         { return len(c.Items) == 0 }
func (c *Cart) TotalCost() int {
	t := 0
	for _, it := range c.Items {
		t += it.Price
	}
	return t
}

type User struct {
	ID      int
	Name    string
	Address string
	Cart    *Cart
}

// ---------- Payment strategy ----------

type PaymentStrategy interface {
	Pay(amount int) error
}

type CreditCard struct{}
type NetBanking struct{}
type UPI struct{}

func (CreditCard) Pay(a int) error { fmt.Printf("paid %d via credit card\n", a); return nil }
func (NetBanking) Pay(a int) error { fmt.Printf("paid %d via net banking\n", a); return nil }
func (UPI) Pay(a int) error        { fmt.Printf("paid %d via UPI\n", a); return nil }

// ---------- Orders ----------

type Order interface {
	Base() *BaseOrder
	Type() string
}

type BaseOrder struct {
	ID            string
	UserID        int
	Restaurant    *Restaurant
	Items         []MenuItem // snapshot copied from the cart
	Total         int
	Payment       PaymentStrategy
	ScheduledTime string // empty for "now" orders
}

func (b *BaseOrder) Base() *BaseOrder { return b }

type DeliveryOrder struct {
	BaseOrder
	Address string
}

func (*DeliveryOrder) Type() string { return "DELIVERY" }

type PickupOrder struct {
	BaseOrder
	RestaurantAddress string
}

func (*PickupOrder) Type() string { return "PICKUP" }

// ---------- Order factories ----------

type OrderFactory interface {
	CreateOrder(id string, u *User, orderType string, pay PaymentStrategy) (Order, error)
}

func buildOrder(id string, u *User, orderType string, pay PaymentStrategy, scheduled string) (Order, error) {
	items := make([]MenuItem, len(u.Cart.Items))
	copy(items, u.Cart.Items)
	base := BaseOrder{
		ID: id, UserID: u.ID, Restaurant: u.Cart.Restaurant,
		Items: items, Total: u.Cart.TotalCost(), Payment: pay, ScheduledTime: scheduled,
	}
	switch orderType {
	case "DELIVERY":
		return &DeliveryOrder{BaseOrder: base, Address: u.Address}, nil
	case "PICKUP":
		return &PickupOrder{BaseOrder: base, RestaurantAddress: u.Cart.Restaurant.Loc}, nil
	}
	return nil, errors.New("unknown order type: " + orderType)
}

type NowOrderFactory struct{}

func (NowOrderFactory) CreateOrder(id string, u *User, t string, p PaymentStrategy) (Order, error) {
	return buildOrder(id, u, t, p, "")
}

type ScheduleOrderFactory struct{ ScheduleTime string }

func (f ScheduleOrderFactory) CreateOrder(id string, u *User, t string, p PaymentStrategy) (Order, error) {
	return buildOrder(id, u, t, p, f.ScheduleTime)
}

// ---------- Repos ----------

type RestaurantRepo struct{ restaurants map[int]*Restaurant }

func NewRestaurantRepo() *RestaurantRepo {
	return &RestaurantRepo{restaurants: map[int]*Restaurant{}}
}
func (r *RestaurantRepo) Save(x *Restaurant) { r.restaurants[x.ID] = x }
func (r *RestaurantRepo) GetAll() []*Restaurant {
	out := make([]*Restaurant, 0, len(r.restaurants))
	for _, x := range r.restaurants {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type OrderRepo struct{ orders map[string]Order }

func NewOrderRepo() *OrderRepo    { return &OrderRepo{orders: map[string]Order{}} }
func (r *OrderRepo) Save(o Order) { r.orders[o.Base().ID] = o }
func (r *OrderRepo) GetByID(id string) (Order, error) {
	o, ok := r.orders[id]
	if !ok {
		return nil, errors.New("order not found: " + id)
	}
	return o, nil
}
func (r *OrderRepo) GetAll() []Order {
	out := make([]Order, 0, len(r.orders))
	for _, o := range r.orders {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Base().ID < out[j].Base().ID })
	return out
}
func (r *OrderRepo) GetByUser(userID int) []Order {
	var out []Order
	for _, o := range r.GetAll() {
		if o.Base().UserID == userID {
			out = append(out, o)
		}
	}
	return out
}

// ---------- Services ----------

type NotificationService struct{}

func (NotificationService) NotifyUser(o Order) {
	b := o.Base()
	fmt.Printf("notify user %d: %s order %s confirmed (total %d)\n", b.UserID, o.Type(), b.ID, b.Total)
}

type RestaurantService struct{ repo *RestaurantRepo }

func NewRestaurantService(r *RestaurantRepo) *RestaurantService { return &RestaurantService{repo: r} }

func (s *RestaurantService) AddRestaurant(r *Restaurant) error {
	if r.Name == "" || len(r.Menu) == 0 {
		return errors.New("restaurant needs a name and a menu")
	}
	s.repo.Save(r)
	return nil
}

func (s *RestaurantService) SearchByLoc(loc string) []*Restaurant {
	var out []*Restaurant
	for _, r := range s.repo.GetAll() {
		if strings.EqualFold(r.Loc, loc) {
			out = append(out, r)
		}
	}
	return out
}

type OrderService struct {
	repo     *OrderRepo
	notifier NotificationService
	seq      int
}

func NewOrderService(r *OrderRepo, n NotificationService) *OrderService {
	return &OrderService{repo: r, notifier: n}
}

func (s *OrderService) PlaceOrder(u *User, orderType string, f OrderFactory, pay PaymentStrategy) (Order, error) {
	if u.Cart == nil || u.Cart.IsEmpty() {
		return nil, errors.New("cart is empty")
	}
	s.seq++
	order, err := f.CreateOrder(fmt.Sprintf("O%03d", s.seq), u, orderType, pay)
	if err != nil {
		return nil, err
	}
	if err := pay.Pay(order.Base().Total); err != nil {
		return nil, err
	}
	s.repo.Save(order)
	s.notifier.NotifyUser(order)
	u.Cart = nil // cart consumed
	return order, nil
}

func (s *OrderService) ListOrders(userID int) []Order { return s.repo.GetByUser(userID) }

// ---------- Demo (the "Tomato" orchestration) ----------

func main() {
	restSvc := NewRestaurantService(NewRestaurantRepo())
	orderSvc := NewOrderService(NewOrderRepo(), NotificationService{})

	pizza := &Restaurant{ID: 1, Name: "Pizza Place", Loc: "Dispur", Menu: []MenuItem{
		{"M1", "Margherita", 250}, {"M2", "Garlic Bread", 120},
	}}
	if err := restSvc.AddRestaurant(pizza); err != nil {
		fmt.Println(err)
		return
	}

	found := restSvc.SearchByLoc("dispur")
	fmt.Println("found:", found[0].Name)

	user := &User{ID: 1, Name: "DS", Address: "Some street", Cart: &Cart{Restaurant: found[0]}}
	user.Cart.AddToCart(found[0].Menu[0])
	user.Cart.AddToCart(found[0].Menu[1])

	if _, err := orderSvc.PlaceOrder(user, "DELIVERY", NowOrderFactory{}, UPI{}); err != nil {
		fmt.Println("error:", err)
	}

	user.Cart = &Cart{Restaurant: found[0]}
	user.Cart.AddToCart(found[0].Menu[0])
	if _, err := orderSvc.PlaceOrder(user, "PICKUP", ScheduleOrderFactory{ScheduleTime: "20:00"}, CreditCard{}); err != nil {
		fmt.Println("error:", err)
	}

	for _, o := range orderSvc.ListOrders(user.ID) {
		fmt.Printf("%s %s total=%d scheduled=%q\n", o.Base().ID, o.Type(), o.Base().Total, o.Base().ScheduledTime)
	}
}
