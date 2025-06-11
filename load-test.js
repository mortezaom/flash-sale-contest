import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = "http://localhost:8080";

export const options = {
  setupTimeout: "30s",
  scenarios: {
    purchase_test: {
      executor: "constant-vus",
      vus: 1100,
      duration: "30s",
    },
  },
};

export function setup() {
  const response = http.get(`${BASE_URL}/sale/info`);
  
  if (response.status !== 200) {
    throw new Error("Failed to get sale info");
  }

  const data = response.json();
  
  // Extract all IDs from first_items and last_items arrays
  const allIds = [];
  
  data.first_items.forEach(item => {
    const match = item.match(/_item_(\d+)$/);
    if (match) allIds.push(parseInt(match[1], 10));
  });
  
  data.last_items.forEach(item => {
    const match = item.match(/_item_(\d+)$/);
    if (match) allIds.push(parseInt(match[1], 10));
  });
  
  // Find actual min and max
  const startId = Math.min(...allIds);
  const endId = Math.max(...allIds);
  
  // Create array of all item IDs
  const allItemIds = [];
  for (let i = startId; i <= endId; i++) {
    allItemIds.push(i);
  }

  return { allItemIds };
}

export default function (data) {
  const userId = `_${__VU}`;
  
  // Randomly pick an item ID from the list
  const randomIndex = Math.floor(Math.random() * data.allItemIds.length);
  const itemId = data.allItemIds[randomIndex];


  // Try checkout
  const checkoutResponse = http.post(
    `${BASE_URL}/checkout?user_id=${userId}&id=${itemId}`
  );

  check(checkoutResponse, {
    "checkout status valid": (r) => [200, 403, 409].includes(r.status),
  });

  if (checkoutResponse.status === 200) {
    const { code } = checkoutResponse.json();
    
    const purchaseResponse = http.post(
      `${BASE_URL}/purchase?code=${code}`
    );

    check(purchaseResponse, {
      "purchase status valid": (r) => [200, 400].includes(r.status),
    });
  }
}