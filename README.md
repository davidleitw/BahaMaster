# BahaMaster

`BahaMaster` 針對巴哈單一大樓製作的基於 LLM API 的全文檢索工具，通常會用在找小夥伴的黑歷史或者~~肉搜~~

`BahaMaster` 的使用流程預想如下，會根據實作過程調整使用者的使用方式

- 在 `.env` 當中寫入巴哈的帳號，密碼(如果爬非場外文則不需要)
- 在 `.env` 中填入欲檢索的大樓資訊，包含 `bsn` 與 `snA`
  - 以資工串舉例，點進大樓後查看網址 https://forum.gamer.com.tw/C.php?page=1&bsn=60076&snA=3146926
  - `bsn` 代表哪個版，60076 為場外編號
  - `snA` 代表文章號碼，具體巴哈姆特官方怎麼存的不得而知，可以假設每篇文章會對應一個 `snA`
- 在 `.env` 中輸入 OPENAI 的 token
- 在輸入完之後會把整個大樓的每層樓，包含留言都整理並且存到本地的 sqlite db
- 接著使用的時候(還沒有確定每個步驟要怎麼給使用者操作)，就可以直接輸入問題，本專案會利用 gpt API 與本地端的 DB 進行交互，獲得想要的答案，可以用口語的方式問問題，像是
  - xxxx 在這個月發了幾次晚餐文 -> 回應次數或者 array of floor
  - oooo 是否曾經提到他在哪個公司上班 -> 如果有提過，回應樓層數或者留言

---

### 為甚麼要做這個專案

這個 Project 雖然看似惡搞，但是充滿著各種優化空間，非常適合作為練習專案
- 因為爬蟲不能太頻繁 -> 在本地端建立 db 儲存，並且收到問題可以先檢查是否有爬過同樣的大樓
- 因為 gpt API 很貴 -> 是否可以 cache 住某些問題，下次問相同的問題即可直接回應

簡單來說，對於某些需要很多成本的操作，都能通過建立類似 cache 性質的架構來避免重複執行

---

### Disclaimer

This web crawler project `BahaMaster` is a practice tool designed to provide users with full-text search capabilities for the Bahamut Forum (hereinafter referred to as "Bahamut"), facilitating users in locating specific content. Users understand and agree to the following disclaimer terms:

1. Practice Project: `BahaMaster` is a project created by the author for practicing web crawling techniques and automation. It is not intended for commercial use or large-scale data extraction. Users should use `BahaMaster` solely for learning and personal research purposes.
2. Assumption of Risk: Since `BahaMaster` requires users to log into Bahamut using their account credentials, the use of this tool may result in the user's Bahamut account being restricted, temporarily banned, or permanently banned. Users are responsible for assuming these risks.
3. Legality: Users agree to comply with all applicable laws and Bahamut's terms of service when using `BahaMaster`. Users bear all responsibility and consequences for any violations of laws or Bahamut's terms of service resulting from their use of BahaMaster.


Disclaimer: The author of this project is not liable for any Bahamut account bans, data loss, or other damages caused by the use of `BahaMaster`. Users understand and accept that all risks and consequences associated with using `BahaMaster` are their own responsibility, and agree not to hold the author of `BahaMaster` liable for any legal liabilities.