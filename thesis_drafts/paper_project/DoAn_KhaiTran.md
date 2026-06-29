# HUST SIMULATOR - ĐỒ ÁN TỐT NGHIỆP



<!-- START OF 0_2_Loi_cam_on.tex (Lời cảm ơn) -->


\pagenumbering{roman}

\begin{center}
    \Large{**LỜI CẢM ƠN**}\\
\end{center}

Trong suốt quá trình học tập và thực hiện đồ án tốt nghiệp, tôi đã nhận được rất nhiều sự quan tâm, giúp đỡ và động viên quý báu từ các thầy cô, gia đình và bạn bè. Đây là nguồn động lực to lớn giúp tôi có thể hoàn thành đề tài một cách tốt nhất.

Trước hết, tôi xin bày tỏ lòng biết ơn sâu sắc tới **TS. Trịnh Thành Trung**, người đã trực tiếp hướng dẫn và tận tình chỉ bảo tôi trong suốt quá trình nghiên cứu và thực hiện đề tài. Những ý kiến đóng góp quý báu, sự hỗ trợ tận tâm cùng những định hướng khoa học của thầy đã giúp tôi từng bước hoàn thiện đồ án này cũng như tích lũy thêm nhiều kiến thức và kinh nghiệm quý giá trong quá trình học tập và nghiên cứu.

Tôi xin chân thành cảm ơn các thầy cô của Trường Đại học Bách khoa Hà Nội nói chung và các thầy cô trong Viện Công nghệ Thông tin và Truyền thông nói riêng đã tận tâm giảng dạy, truyền đạt những kiến thức nền tảng và chuyên môn quý báu trong suốt những năm học vừa qua. Những kiến thức đó chính là hành trang quan trọng để tôi có thể thực hiện và hoàn thành đề tài tốt nghiệp này.

Tôi cũng xin gửi lời cảm ơn sâu sắc tới gia đình, những người luôn yêu thương, động viên và tạo mọi điều kiện thuận lợi cả về vật chất lẫn tinh thần để tôi có thể yên tâm học tập và hoàn thành đồ án đúng tiến độ. Bên cạnh đó, tôi xin cảm ơn bạn bè và các anh chị đã luôn đồng hành, chia sẻ kinh nghiệm, hỗ trợ và khích lệ tôi trong suốt quá trình học tập cũng như thực hiện đề tài.

Đồ án này là kết quả của quá trình học tập, nghiên cứu và rèn luyện tại Trường Đại học Bách khoa Hà Nội. Một lần nữa, tôi xin chân thành cảm ơn tất cả sự giúp đỡ, động viên và đồng hành quý báu mà mọi người đã dành cho tôi.


<!-- END OF 0_2_Loi_cam_on.tex -->
---


<!-- START OF 0_3_Tom_tat_noi_dung.tex (Tóm tắt nội dung) -->



\begin{center}
    \Large{**TÓM TẮT NỘI DUNG ĐỒ ÁN**}\\
\end{center}

**Abstract**

A Digital Twin is a virtual representation of a physical environment that continuously reflects its real-world state through streamed and updated data. To accurately visualize such an environment, a well-structured data infrastructure is essential, requiring raw data to be transformed and standardized before being rendered in the virtual space. While static entities, such as buildings and road networks, can be collected relatively easily, acquiring dynamic information about users' locations and behaviors presents a greater challenge. Rather than relying on users to actively provide their data, an automated and unobtrusive data collection mechanism is required.

The challenge becomes even more significant in real-time Digital Twin systems, particularly in spatial data processing. Although modern real-time communication technologies have advanced considerably, continuously exchanging bidirectional location data between clients and servers can impose substantial bandwidth and computational overhead as the number of concurrent users increases. Without an efficient spatial communication architecture, the server can quickly become overloaded, resulting in degraded system performance.

To address these challenges, this thesis presents **Hust Simulator**, a real-time data collection and behavior analysis system for Digital Twin applications, deployed within the campus of Hanoi University of Science and Technology. A university campus was selected as the initial deployment environment because of its high user density and frequent movement between classrooms and learning facilities, providing an ideal scenario for validating both data quality and system performance. Although evaluated in an academic environment, the proposed framework is readily extensible to larger-scale spatial computing applications, including smart transportation, smart cities, and other Digital Twin systems.

Beyond data collection, the primary contribution of the proposed system lies in transforming large volumes of raw mobility data into meaningful knowledge through structured storage and advanced analytical methods. The resulting insights can support campus management, optimize operational efficiency, and provide a practical foundation for future large-scale intelligent transportation and Digital Twin applications.

\begin{flushright}
Sinh viên thực hiện\\
\begin{tabular}{@{}c@{}}
Trần Mạnh Khải
\end{tabular}
\end{flushright}



<!-- END OF 0_3_Tom_tat_noi_dung.tex -->
---


<!-- START OF 0_4_Tom_tat_noi_dung_English.tex (Abstract) -->



\begin{center}
    \Large{**ABSTRACT**}\\
\end{center}

A Digital Twin is a digital model that reflects the state of a real-world environment through continuously collected and updated data. To build a Digital Twin of practical value, the system must not only store data but also gather information from real-world sources, synchronize data in real time, and maintain interactivity among multiple users within the same digital space. As the number of users scales up, transmitting spatial data, managing concurrent connections, and processing location-based queries become critical architectural challenges.

Beyond data synchronization requirements, a complete Digital Twin also needs the ability to form behavioral models from collected data. Through users' movement history and interactions, the system can construct datasets that support journey analysis, space utilization assessment, and future predictive tasks. This demands a backend platform capable of efficiently receiving, aggregating, and processing data from multiple client devices.

Motivated by these requirements, this thesis develops **Hust Simulator** --- a technical framework for a Digital Twin system within the Hanoi University of Science and Technology campus. The system enables location data collection from multiple simultaneous users, performs real-time spatial data synchronization, and stores movement history to support analytical functions. Limiting the research scope to the university campus facilitates data collection, verification, and evaluation through direct access to the real-world environment. Moreover, this is a high-density user area with continuous movement activities occurring between lecture halls, study areas, the library, and other functional zones. These characteristics generate a rich volume of spatial data reflecting diverse user behaviors and movement patterns, making it well-suited for testing data collection mechanisms, real-time synchronization, and journey analysis within the Digital Twin system.

This platform not only serves the Digital Twin use case within a university campus but can also be extended to systems requiring large-scale spatial data processing, such as intelligent transportation, smart cities, and other Digital Twin applications.
\begin{flushright}
Author\\
\begin{tabular}{@{}c@{}}
Trần Mạnh Khải
\end{tabular}
\end{flushright}



<!-- END OF 0_4_Tom_tat_noi_dung_English.tex -->
---


<!-- START OF 0_5_Danh_muc_viet_tat.tex (Danh mục viết tắt) -->



\begin{longtable}{l p{5.5cm} p{6.5cm}}
	\hline
   **Viết tắt**  & **Tên tiếng Anh**	& **Tên tiếng Việt** \\ \hline
	**AOI** & Area of Interest & Vùng quan tâm\\
	**API** & Application Programming Interface & Giao diện lập trình ứng dụng\\
	**CPU** & Central Processing Unit & Bộ xử lý trung tâm\\
	**GPS** & Global Positioning System & Hệ thống định vị toàn cầu\\
	**GPU** & Graphics Processing Unit & Bộ xử lý đồ họa\\
	**gRPC** & gRPC Remote Procedure Calls & Giao thức gọi hàm từ xa gRPC\\
	**HTTP** & Hypertext Transfer Protocol & Giao thức truyền tải siêu văn bản\\
	**JTS** & Java Topology Suite & Bộ công cụ hình học Java\\
	**JVM** & Java Virtual Machine & Máy ảo Java\\
	**JWT** & JSON Web Token & Mã thông báo Web JSON\\
	**ORM** & Object-Relational Mapping & Ánh xạ đối tượng -- quan hệ\\
	**POI** & Point of Interest & Điểm quan tâm\\
	**RAM** & Random Access Memory & Bộ nhớ truy cập ngẫu nhiên\\
	**REST** & Representational State Transfer & Chuyển giao trạng thái đại diện\\
	**SSL/TLS** & Secure Sockets Layer / Transport Layer Security & Giao thức bảo mật lớp mạng\\
	**STTF** & Spatio-Temporal Transformer Fusion & Kiến trúc Transformer Không gian -- Thời gian\\
	**WebRTC** & Web Real-Time Communication & Giao tiếp thời gian thực trên nền web\\
    \hline
\end{longtable}



<!-- END OF 0_5_Danh_muc_viet_tat.tex -->
---


<!-- START OF 0_6_Thuat_ngu.tex (Danh mục thuật ngữ) -->



\begin{longtable}{p{4cm} p{10cm}}
	\hline
    **Thuật ngữ** & **Ý nghĩa** \\ \hline
    **Digital Twin** & Bản sao số của thực thể vật lý trong không gian mạng \\
    **Check-in** & Điểm dừng mà sinh viên ghé qua trong quá trình di chuyển trong khuôn viên trường \\
    **Context-Aware Recommender** & Hệ thống dự báo điểm đến dựa trên sự kết hợp giữa thông tin ngữ cảnh và sở thích cá nhân \\
    **Database-per-Service** & Mô hình kiến trúc phân tán trong đó mỗi dịch vụ (service) sở hữu và quản lý một cơ sở dữ liệu độc lập \\
    **Flocking Noise** & Kỹ thuật tạo nhiễu không gian để mô phỏng sự di chuyển ngẫu nhiên và tự nhiên của đám đông \\
    **Harmonic Flocking** & Thuật toán dịch chuyển các vùng nhiễu theo biến thời gian nhằm tái tạo chân thực dòng người đi lại \\
    **Heatmap** & Bản đồ biểu diễn mật độ hoặc cường độ phân bố của dữ liệu theo không gian \\
    **Microservices** & Kiến trúc phần mềm trong đó hệ thống được chia thành nhiều dịch vụ nhỏ hoạt động độc lập \\
    **Polyglot Persistence** & Mô hình kết hợp sử dụng nhiều loại hệ quản trị cơ sở dữ liệu khác nhau nhằm tối ưu hóa hiệu năng lưu trữ \\
    **Predictive Heatmap** & Bản đồ biểu diễn mật độ dự báo về sự phân bố của đám đông tại một thời điểm trong tương lai \\
    **Pub/Sub** & Cơ chế giao tiếp (Publish-Subscribe) thông qua việc phát và đăng ký nhận thông điệp giữa các thiết bị \\
    **Simulation Mode** & Chế độ giả lập cho phép thay đổi dữ liệu ngữ cảnh toàn cục để phân tích kịch bản và rủi ro \\
    **Spatial Sharding** & Kỹ thuật phân chia hệ thống quản lý dữ liệu theo từng khu vực không gian nhằm tăng khả năng mở rộng \\
    \hline
\end{longtable}


<!-- END OF 0_6_Thuat_ngu.tex -->
---


<!-- START OF 1_Gioi_thieu.tex (Chương 1: Giới thiệu đề tài) -->



Chương này trình bày tổng quan về đề tài, xác định mục tiêu, phạm vi nghiên cứu và đề xuất định hướng giải pháp kỹ thuật dựa trên kiến trúc hiện đại.

## Đặt vấn đề

Để tích hợp hành vi người dùng vào hệ thống Digital Twin không phải là một bài toán đơn giản. Một hệ thống Digital Twin nếu chỉ dừng lại ở việc trực quan hóa các thực thể tĩnh thì sẽ không có sự khác biệt đáng kể so với các mô hình 3D đồ họa thông thường. Do đó, việc đưa yếu tố con người – với các hoạt động diễn ra theo thời gian thực – vào không gian số là điều kiện tiên quyết để xây dựng một Digital Twin hoàn chỉnh. Nhưng làm thế nào để có thể đưa hành vi người dùng trở thành dữ liệu số? Một số dữ liệu hành vi của người dùng có thể kể đến như vị trí, lịch sử di chuyển và tương tác với không gian vật lý, tương tác giữa người và người chính là những thành phần cốt lõi giúp bản sao số phản ánh chính xác trạng thái vận động của thế giới thực [isprs]. Từ đây, thách thức kỹ thuật đầu tiên cần phải giải quyết nằm ở khâu xử lý dữ liệu: làm thế nào để tổ chức và chuẩn hóa khối lượng lớn dữ liệu hành vi thô kể trên thành một mô hình dữ liệu có cấu trúc, làm nền tảng vững chắc cho quá trình trực quan hóa.

Thách thức thứ hai xuất hiện trong khâu xử lý luồng dữ liệu không gian theo thời gian thực. Việc đồng bộ vị trí liên tục giữa hàng loạt người dùng trong cùng một hệ sinh thái không thể thực hiện một cách ngây thơ được. Đối với hệ thống Digital Twin thời gian thực, dữ liệu cần được cập nhật liên tục để duy trì độ chính xác của thế giới thực, nhưng đồng thời hệ thống phải tối ưu hóa nhằm hạn chế việc truyền tải dư thừa, giảm tải chi phí băng thông và tài nguyên tính toán [realtimesync,dtsync]. Khi quy mô người dùng mở rộng, số lượng kết nối đồng thời và lượng thông điệp trao đổi giữa thiết bị khách và máy chủ sẽ tăng theo cấp số nhân. Nếu hệ thống áp dụng cơ chế phân phối mọi thay đổi trạng thái đến toàn bộ thiết bị trong mạng, khối lượng dữ liệu cần xử lý sẽ bùng nổ, gây ra độ trễ lớn và làm suy giảm nghiêm trọng khả năng đáp ứng. Vấn đề đặt ra ở đây là: Cần thiết kế một cơ chế kiến trúc và đồng bộ như thế nào để hệ thống vượt qua được điểm nghẽn về giới hạn hạ tầng mạng và năng lực máy chủ?

Cuối cùng, trên cơ sở khối lượng dữ liệu khổng lồ được thu thập, hệ thống cần có năng lực tổng hợp và trích xuất thành tri thức có giá trị [dtresearch]. Giá trị thực tiễn của một hệ thống Digital Twin không chỉ nằm ở khả năng tái hiện không gian ở thời điểm hiện tại, mà còn ở tiềm năng khai thác kho dữ liệu tích lũy trong suốt quá trình vận hành để đưa vào phân tích. Nếu các dữ liệu về hành vi, vị trí và tương tác chỉ được dùng để phục vụ mục đích hiển thị, hệ thống sẽ lãng phí một nguồn tài nguyên thông tin lớn. Mặc dù nhiều hệ thống Digital Twin đã đạt được khả năng trực quan hóa, việc tích hợp khai thác dữ liệu phục vụ phân tích chuyên sâu vẫn là một trong những thách thức quan trọng đối với Digital Twin [sharma2022digitaltwins]. Do đó, yêu cầu cấp thiết thứ ba là phải xây dựng các phương pháp phân tích dữ liệu chuyên sâu để lập mô hình hành vi, từ đó phục vụ cho các bài toán dự đoán, quy hoạch không gian và hỗ trợ ra quyết định trong tương lai.

## Mục tiêu và phạm vi đề tài

Xuất phát từ những thách thức và yêu cầu thực tiễn đã đặt ra, tôi hướng tới mục tiêu xây dựng hệ thống **Hust Simulator** --- một nền tảng kiến trúc lõi phục vụ việc thu thập, phân phối và khai thác dữ liệu không gian trong khuôn viên đại học Bách Khoa Hà Nội. Các mục tiêu cụ thể bao gồm:

-  **Thiết kế cơ chế thu thập:** Xây dựng luồng thu thập dữ liệu vị trí, trạng thái và hành vi từ các thiết bị đầu cuối trong môi trường đa người dùng.

-  **Tối ưu hóa phân phối thời gian thực:** Phát triển kỹ thuật phân phối luồng dữ liệu không gian theo thời gian thực, giải quyết bài toán quá tải tài nguyên máy chủ khi lượng lớn dữ liệu vị trí được cập nhật và trao đổi liên tục với tần suất cao.

-  **Xây dựng nền tảng quản trị:** Phát triển hệ thống quản trị nhằm quản lý và phân tích hiệu quả nguồn dữ liệu sinh ra từ tương tác của người dùng.

-  **Khai thác và phân tích dữ liệu:** Tích hợp các phương pháp phân tích trên tập dữ liệu lịch sử nhằm lập mô hình và dự đoán hành vi, từ đó trích xuất tri thức hỗ trợ công tác quản lý, quy hoạch và vận hành không gian.

-  **Đảm bảo khả năng mở rộng:** Xây dựng kiến trúc hệ thống theo mô hình phân tán, đảm bảo tính linh hoạt và khả năng mở rộng đáp ứng sự gia tăng về quy mô người dùng cũng như diện tích môi trường mô phỏng.

Đồ án tập trung chủ yếu vào việc thiết kế và hoàn thiện bộ khung kiến trúc nền tảng. Các hạng mục trọng tâm được giới hạn trong việc hiện thực hóa cơ chế truyền tải dữ liệu thời gian thực, xây dựng hệ thống quản trị và phát triển các luồng phân tích dữ liệu thu thập được. Với ưu tiên giải quyết các bài toán về hiệu năng xử lý và khả năng chịu tải, hệ thống được định hướng xây dựng như một nền tảng lõi độc lập, đảm bảo tính sẵn sàng tích hợp cao, cung cấp nền tảng vững chắc để các nhà phát triển có thể dễ dàng kết nối và tùy biến giao diện đồ họa của riêng họ lên trên bộ khung này.

## Định hướng giải pháp

Đối với bài toán thu thập dữ liệu hành vi, thách thức lớn nhất không nằm ở khía cạnh kỹ thuật mà ở việc tạo ra một cơ chế đủ hấp dẫn để người dùng chủ động tương tác và chia sẻ dữ liệu vị trí. Quan sát các nền tảng công nghệ phổ biến cho thấy, dữ liệu hành vi thường được thu thập thông qua một dịch vụ cốt lõi mang lại giá trị trực tiếp cho người dùng. Chẳng hạn, Google Maps thu thập dữ liệu thông qua chức năng dẫn đường, Strava thông qua hoạt động theo dõi luyện tập thể thao, hay Pokémon GO thông qua trải nghiệm trò chơi thực tế tăng cường. Tương tự, trong hệ thống của tôi, vai trò này được đảm nhiệm bởi một nền tảng mạng xã hội được xây dựng trong không gian số của trường đại học. Nền tảng này vừa tạo môi trường tương tác cho người dùng, vừa cung cấp nguồn dữ liệu hành vi phục vụ quá trình mô phỏng và phân tích. Hệ thống được xây dựng theo hai giai đoạn. Trước hết, cần hình thành một bản sao số của môi trường thực bằng cách xây dựng các dữ liệu tĩnh như cấu trúc tòa nhà, mạng lưới đường đi và tích hợp các data pipeline để liên tục đồng bộ các thông tin động từ thế giới thực, chẳng hạn như lịch học hay các sự kiện hướng nghiệp. Nhờ đó, không gian số luôn phản ánh tương đối chính xác trạng thái của môi trường thực tế. Sau khi hoàn thiện phần nền tảng này, hệ thống cung cấp cho người dùng các chức năng tương tác với không gian vật lý, tham gia sự kiện và giao tiếp với nhau thông qua một mạng xã hội tích hợp. Trong quá trình sử dụng các chức năng của hệ thống, dữ liệu về hành vi và mức độ tương tác của người dùng được ghi nhận một cách thụ động thông qua các hoạt động diễn ra hằng ngày, thay vì yêu cầu người dùng chủ động cung cấp thông tin. Điều này giúp tạo ra nguồn dữ liệu phản ánh sát hơn thực tế, phục vụ cho các bài toán phân tích và mô phỏng trong các giai đoạn tiếp theo.

Đối với bài toán đồng bộ vị trí liên tục, hệ thống áp dụng cơ chế truyền dữ liệu theo phạm vi lân cận. Theo đó, thông tin vị trí của một người dùng chỉ được cập nhật tới các thiết bị khách đang ở gần hoặc có liên quan về mặt không gian, thay vì gửi đồng thời tới toàn bộ người dùng trong hệ thống. Bằng cách tiếp cận này, mỗi người dùng chỉ nhận được thông tin cập nhật từ các đối tượng nằm trong phạm vi quan tâm của mình. Điều này giúp loại bỏ lượng dữ liệu truyền tải thừa, giảm tải cho máy chủ và giúp hệ thống dễ dàng mở rộng khi số lượng người dùng tăng cao.

Cuối cùng, đối với nhu cầu phân tích và khai thác dữ liệu, hệ thống định hướng xây dựng một kiến trúc lưu trữ và tổ chức dữ liệu chặt chẽ, luồng dữ liệu thô sau khi thu thập sẽ trải qua quá trình tổng hợp, làm sạch và chuẩn hóa để tạo thành nguồn tập dữ liệu có cấu trúc tối ưu. Cơ sở dữ liệu này đóng vai trò then chốt trong việc huấn luyện các mô hình máy học nhằm giải quyết bài toán dự đoán điểm đến tiếp theo của từng cá nhân dựa trên chuỗi lịch sử di chuyển. Từ kết quả dự đoán ở cấp độ vi mô này, hệ thống sẽ tổng hợp, ngoại suy và trực quan hóa thành bản đồ mật độ dự đoán cho toàn bộ khuôn viên. Chức năng này cho phép các nhà quản lý nhìn trước được xu hướng dịch chuyển, cảnh báo sớm các điểm nóng tập trung đông người để từ đó đưa ra quyết định điều phối và quy hoạch không gian hiệu quả.

## Bố cục đồ án

Phần còn lại của báo cáo được tổ chức thành 6 chương.

Chương 2 trình bày quá trình khảo sát hiện trạng, phân tích yêu cầu và xây dựng các mô hình nghiệp vụ của hệ thống.

Chương 3 giới thiệu cơ sở lý thuyết và các công nghệ được sử dụng trong đồ án.

Chương 4 trình bày quá trình phân tích và thiết kế hệ thống, bao gồm kiến trúc tổng thể, cơ sở dữ liệu và các thành phần chức năng chính.

Chương 5 trình bày quá trình triển khai thực tế, kết quả thử nghiệm và đánh giá hệ thống.

Chương 6 tập trung phân tích các giải pháp đóng góp của đề tài, các kỹ thuật xử lý và khai thác dữ liệu không gian thời gian thực.

Cuối cùng, Chương 7 tổng kết những kết quả đạt được, chỉ ra các hạn chế còn tồn tại và đề xuất hướng phát triển trong tương lai.



<!-- END OF 1_Gioi_thieu.tex -->
---


<!-- START OF 2_Khao_sat.tex (Chương 2: Khảo sát và phân tích yêu cầu) -->



Chương này trình bày quá trình khảo sát và phân tích yêu cầu của hệ thống Digital Twin. Nội dung chương tập trung khảo sát hiện trạng các hệ thống liên quan, phân tích các bài toán còn tồn tại và xác định các yêu cầu cần thiết cho hệ thống đề xuất. Bên cạnh đó, chương cũng trình bày tổng quan chức năng của hệ thống thông qua biểu đồ use case, đặc tả các chức năng quan trọng và xác định các yêu cầu phi chức năng và khả năng mở rộng.

## Khảo sát hiện trạng

Để đánh giá các hệ thống giải quyết bài toán tương tự, đề tài sử dụng bốn tiêu chí chính: phạm vi hệ thống, khả năng tương tác không gian số, khả năng tương tác giữa nhiều người dùng, và khả năng phân tích dữ liệu. Riêng về kỹ thuật xử lý thời gian thực, do phần lớn các hệ thống không công bố chi tiết kỹ thuật sử dụng, phần này chỉ dừng lại ở việc đánh giá mức độ ứng dụng thời gian thực trong từng hệ thống, thay vì đi sâu vào cơ chế triển khai.

Qua khảo sát, có thể thấy nhiều nền tảng hiện nay đã đạt được những kết quả nhất định trong việc thu thập dữ liệu vị trí, hỗ trợ tương tác dựa trên không gian, hoặc khai thác dữ liệu phục vụ phân tích. Tuy nhiên, các khả năng này thường được phát triển riêng biệt, hướng tới những mục tiêu khác nhau. Để có cái nhìn tổng quan, phần này sẽ phân tích cách tiếp cận của một số hệ thống tiêu biểu hiện nay đối với từng bài toán cụ thể, bao gồm Google Maps, Strava, Pokémon Go và Zenly.

![Google Maps](Hinhve/GoogleMap.png)
*Hình: Google Maps*

Đầu tiên là Google Maps [googlemaps], ứng dụng tiên phong trong lĩnh vực bản đồ và chia sẻ vị trí. Nền tảng này thu thập dữ liệu vị trí người dùng một cách tự nhiên thông qua nhu cầu tìm đường, từ đó dữ liệu thu được được khai thác cho nhiều mục đích phân tích khác nhau, tiêu biểu là tính năng dự đoán thời gian di chuyển. Tuy nhiên, Google Maps về bản chất vẫn là một ứng dụng phục vụ nhu cầu cá nhân, chưa hướng tới việc tạo ra tương tác trực tiếp giữa người dùng với nhau.

Các ứng dụng ra đời sau đã bổ sung yếu tố này bằng cách tích hợp khả năng giao tiếp và chia sẻ giữa người dùng. Strava [strava] là một ứng dụng theo dõi và chia sẻ hoạt động thể thao (chạy bộ, đạp xe...), cho phép người dùng so sánh thành tích và tương tác với bạn bè trong cộng đồng. Pokémon Go khai thác dữ liệu vị trí theo hướng trò chơi thực tế tăng cường, kết hợp việc thu thập Pokémon ngoài đời thực với khả năng chia sẻ thành tích giữa người chơi. Trong khi đó, Zenly [zenly] tập trung vào việc chia sẻ vị trí và trạng thái hiện tại của bạn bè theo thời gian thực, đặt trọng tâm vào yếu tố xã hội hơn là chức năng định vị thuần túy. Có thể thấy mỗi ứng dụng lựa chọn một phương thức thu thập dữ liệu người dùng riêng biệt, phù hợp với domain mà mình hướng tới. Sự khác biệt về không gian hoạt động --- từ phạm vi, quy mô cho đến yêu cầu về tính thời gian thực --- chính là yếu tố quyết định mô hình kỹ thuật mà mỗi hệ thống lựa chọn để triển khai. Phần tiếp theo sẽ đi sâu phân tích cách thức cụ thể mà từng hệ thống đã thực hiện.

\begin{table}[H]
\centering
\caption{So sánh chức năng giữa một số hệ thống liên quan}

\renewcommand{\arraystretch}{1.6}
\resizebox{\textwidth}{!}{
\begin{tabular}{p{4.5cm}p{3.2cm}p{3.2cm}p{3.2cm}p{3.2cm}}
\toprule
**Chức năng**
& **Google Maps**
& **Strava**
& **Zenly**
& **Pokémon GO** \\
\midrule
Chức năng chính
& Tìm đường, điều hướng cá nhân
& Thể thao, vận động
& Mạng xã hội định vị bạn bè
& Game AR \\
Thu thập dữ liệu vị trí
& Có & Có & Có & Có \\
Chia sẻ vị trí theo thời gian thực
& Một phần & Không & Có & Hạn chế \\
Quan sát trạng thái người khác theo thời gian thực
& Hạn chế & Không & Có & Hạn chế \\
Người dùng tương tác trong không gian số
& Không & Không & Có & Có \\
Yêu cầu thời gian thực
& Một phần & Không & Có & Một phần \\
Phản ánh trạng thái tổng thể của môi trường
& Hạn chế & Không & Hạn chế & Hạn chế \\
Hỗ trợ ra quyết định
& Một phần & Hạn chế & Không & Không \\
\bottomrule
\end{tabular}}
\end{table}

Có thể thấy điểm chung của các hệ thống nêu trên là đều được thiết kế cho một domain có quy mô không gian rộng và đối tượng sử dụng đa dạng --- từ phạm vi toàn cầu như Google Maps, phạm vi cộng đồng phân tán như Zenly, Strava, cho đến phạm vi trò chơi không ràng buộc địa lý cụ thể như Pokémon GO. Khi bài toán được thu hẹp về một không gian nhỏ hơn với mật độ người dùng cao và đòi hỏi tính thời gian thực thì các mô hình thiết kế này chưa hẳn là lựa chọn tối ưu. Đặc điểm này không chỉ xuất hiện trong môi trường giáo dục mà còn phổ biến ở nhiều lĩnh vực khác như giao thông thông minh trong một khu vực, quản lý sự kiện quy mô lớn hay các bài toán quản lý người dân. Điểm chung của các bài toán này là nhu cầu về một bộ khung đủ linh hoạt để thu thập, đồng bộ và cung cấp dữ liệu vị trí theo thời gian thực, đóng vai trò như lớp hạ tầng dữ liệu cho các hệ thống phân tích và trực quan hóa phía trên. Nhờ đó, các ứng dụng cụ thể có thể tận dụng lại cùng một nền tảng dữ liệu thay vì phải xây dựng lại toàn bộ cơ chế xử lý cho từng bài toán riêng lẻ. Đó cũng chính là động lực để hình thành ý tưởng xây dựng hệ thống Hust Simulator.

## Tổng quan chức năng

Hệ thống Hust Simulator được thiết kế nhằm hỗ trợ môi trường tương tác đa
người dùng theo thời gian thực trong Digital Twin. Hệ thống được phân chia thành
hai nhóm tác nhân chính: Người dùng thông thường (thao tác qua ứng dụng web/di
động) và Quản trị viên (quản lý qua trang tổng quan).

### Biểu đồ use case tổng quát

Biểu đồ use case tổng quát thể hiện cái nhìn toàn cảnh về các nhóm chức năng
chính của hệ thống.

![Biểu đồ use case tổng quát của hệ thống Hust Simulator](Hinhve/UCTongQuat.png)
*Hình: Biểu đồ use case tổng quát của hệ thống Hust Simulator*

Hệ thống được chia thành hai nhóm dịch vụ chính:

    
-  **User Services** (Dành cho Người dùng): Bao gồm Xác thực tài khoản, Tương tác bản đồ số, Tương tác mạng xã hội, và Quản lý hành trình.
    
-  **Administration Services** (Dành cho Quản trị viên): Bao gồm Quản trị hệ thống \& người dùng, Giám sát mật độ thời gian thực, Dự đoán mật độ trong tương lai, và Chạy mô phỏng Simulation.

### Biểu đồ use case phân rã

Hệ thống được chia thành hai nhóm dịch vụ chính. Nhóm dịch vụ dành cho sinh viên bao gồm các chức năng cốt lõi như xác thực tài khoản, tương tác không gian số, tham gia mạng xã hội và quản lý hành trình cá nhân. Nhóm dịch vụ dành cho quản trị viên cung cấp các công cụ để điều hành hệ thống, giám sát và dự báo mật độ đám đông, cũng như chạy các kịch bản mô phỏng.

Dưới đây là chi tiết biểu đồ use case phân rã cho từng chức năng cốt lõi của hệ thống, đi kèm với mô tả ngắn gọn về các tính năng tương ứng.

\noindent{**Nhóm dịch vụ dành cho sinh viên**}

#### Xác thực tài khoản

![Biểu đồ phân rã Use Case Xác thực tài khoản](Hinhve/UC_XacThucTaiKhoan.png)
*Hình: Biểu đồ phân rã Use Case Xác thực tài khoản*

#### Tham gia khuôn viên số

![Biểu đồ phân rã Use Case Tham gia khuôn viên số](Hinhve/UC_ThamGiaKhuonVienSo.png)
*Hình: Biểu đồ phân rã Use Case Tham gia khuôn viên số*

#### Tham gia mạng xã hội

![Biểu đồ phân rã Use Case Tham gia mạng xã hội](Hinhve/UC_MangXaHoi.png)
*Hình: Biểu đồ phân rã Use Case Tham gia mạng xã hội*

#### Trao đổi tin nhắn

![Biểu đồ phân rã Use Case Trao đổi tin nhắn](Hinhve/UC_TraoDoiTinNhan.png)
*Hình: Biểu đồ phân rã Use Case Trao đổi tin nhắn*

#### Lưu trữ và mô phỏng lại hành trình

![Biểu đồ phân rã Use Case Lưu trữ và mô phỏng lại hành trình](Hinhve/UC_QuanLyHanhTrinh.png)
*Hình: Biểu đồ phân rã Use Case Lưu trữ và mô phỏng lại hành trình*

#### Theo dõi sự kiện

![Biểu đồ phân rã Use Case Theo dõi sự kiện](Hinhve/UC_TheoDoiSuKien.png)
*Hình: Biểu đồ phân rã Use Case Theo dõi sự kiện*

#### Tham gia lớp học ảo

![Biểu đồ phân rã Use Case Tham gia lớp học ảo](Hinhve/UC_ThamGiaLopHocAo.png)
*Hình: Biểu đồ phân rã Use Case Tham gia lớp học ảo*

\noindent{**Nhóm dịch vụ dành cho quản trị viên**}

#### Điều hành hệ thống \& tài khoản

![Biểu đồ phân rã Use Case Điều hành hệ thống \& tài khoản](Hinhve/UC_QuanTriHeThong.png)
*Hình: Biểu đồ phân rã Use Case Điều hành hệ thống \& tài khoản*

#### Giám sát mật độ thời gian thực

![Biểu đồ phân rã Use Case Giám sát mật độ thời gian thực](Hinhve/UC_GiamSatThoiGianThuc.png)
*Hình: Biểu đồ phân rã Use Case Giám sát mật độ thời gian thực*

#### Dự báo mật độ trong tương lai

![Biểu đồ phân rã Use Case Dự báo mật độ trong tương lai](Hinhve/UC_DuDoanMatDo.png)
*Hình: Biểu đồ phân rã Use Case Dự báo mật độ trong tương lai*

#### Khởi chạy mô phỏng kịch bản

![Biểu đồ phân rã Use Case Khởi chạy mô phỏng kịch bản](Hinhve/UC_MoPhongWhatIf.png)
*Hình: Biểu đồ phân rã Use Case Khởi chạy mô phỏng kịch bản*

### Quy trình nghiệp vụ

Dưới đây là các quy trình nghiệp vụ cốt lõi của hệ thống để có thể hình dung ra bức tranh hệ thống một cách chi tiết hơn

#### Quy trình đồng bộ trạng thái không gian

Quy trình này mô tả cách hệ thống đồng bộ vị trí và trạng thái của người chơi cho những người dùng lân cận thông qua mô hình AOI Publish/Subscribe, đảm bảo khả năng mở rộng và tính thời gian thực.

![Biểu đồ tuần tự quy trình đồng bộ trạng thái không gian](Hinhve/seq-AOI-pubsub.png)
*Hình: Biểu đồ tuần tự quy trình đồng bộ trạng thái không gian*

#### Quy trình phát hiện điểm dừng và tổng hợp hành trình

Quy trình này mô tả việc thu thập dữ liệu GPS thô của người dùng ở chế độ nền, sau đó hệ thống sẽ xử lý batch processing, lọc nhiễu và phát hiện các điểm dừng chân bằng thuật toán Stop Detection để tổng hợp thành một nhật ký hành trình.

![Biểu đồ tuần tự quy trình tổng hợp nhật ký hành trình](Hinhve/seq-journey.png)
*Hình: Biểu đồ tuần tự quy trình tổng hợp nhật ký hành trình*

## Đặc tả chức năng

### Đặc tả use case: Đăng bài viết gắn vị trí

\begin{table}[H]
    \centering
    \caption{Đặc tả use case: Đăng bài viết gắn vị trí campus}
    
    \renewcommand{\arraystretch}{1.3}
    \begin{tabular}{|p{3.5cm}|p{11cm}|}
        \hline
        **Tên use case** & **Đăng bài viết gắn vị trí campus** \\
        \hline
        **Mã use case** & UC-01 \\
        \hline
        **Tác nhân** & Sinh viên (đã đăng nhập) \\
        \hline
        **Mô tả ngắn gọn** & Cho phép sinh viên đăng tải bài viết và đính kèm vị trí trên bản đồ khuôn viên. \\
        \hline
        **Tiền điều kiện** & Sinh viên đã xác thực thành công. Ứng dụng đã cấp quyền truy cập vị trí và camera. \\
        \hline
        **Hậu điều kiện** & Bài đăng được lưu vào cơ sở dữ liệu, xuất hiện trên feed của những người theo dõi, và được đánh dấu vị trí trên bản đồ campus. \\
        \hline
        **Luồng sự kiện chính** & 
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  Sinh viên nhấn nút tạo bài đăng trên màn hình chính.
            
-  Ứng dụng hiển thị màn hình soạn thảo bài đăng.
            
-  Sinh viên nhập nội dung văn bản, tuỳ chọn thêm hình ảnh hoặc video.
            
-  Sinh viên chọn vị trí campus bằng cách nhấn nút ``Chọn địa điểm'' và chọn tòa nhà hoặc khu vực trên bản đồ.
            
-  Sinh viên nhấn nút ``Đăng'' để xác nhận.
            
-  Hệ thống lưu bài đăng và gắn thông tin vị trí.
            
-  Hệ thống hiển thị thông báo đăng bài thành công và cập nhật feed.
          \\
        \hline
        **Luồng sự kiện phát sinh** &
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  *F1 --- Thiếu kết nối mạng:* Nếu mạng không ổn định, hệ thống thông báo lỗi và yêu cầu thử lại. Nội dung bài đăng được giữ nguyên trong form.
            
-  *F2 --- File quá lớn:* Nếu hình ảnh hoặc video vượt quá giới hạn cho phép, hệ thống hiển thị cảnh báo và yêu cầu chọn lại tệp phù hợp.
          \\
        \hline
    \end{tabular}
\end{table}

### Đặc tả use case: Xem bản đồ campus thời gian thực

\begin{table}[H]
    \centering
    \caption{Đặc tả use case: Xem bản đồ campus và vị trí người dùng xung quanh}
    
    \renewcommand{\arraystretch}{1.3}
    \begin{tabular}{|p{3.5cm}|p{11cm}|}
        \hline
        **Tên use case** & **Xem bản đồ campus và vị trí người dùng xung quanh** \\
        \hline
        **Mã use case** & UC-02 \\
        \hline
        **Tác nhân** & Sinh viên (đã đăng nhập) \\
        \hline
        **Mô tả ngắn gọn** & Cho phép sinh viên xem bản đồ thời gian thực và tương tác với các sinh viên khác ở gần. \\
        \hline
        **Tiền điều kiện** & Sinh viên đã xác thực. Thiết bị đã bật GPS và ứng dụng có quyền truy cập vị trí. \\
        \hline
        **Hậu điều kiện** & Sinh viên thấy bản đồ campus với vị trí của mình và những người dùng trong bán kính 50 mét. \\
        \hline
        **Luồng sự kiện chính** & 
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  Sinh viên chọn tab bản đồ trên thanh điều hướng chính.
            
-  Ứng dụng kết nối WebSocket đến game server.
            
-  Ứng dụng gửi sự kiện \texttt{user:join} kèm JWT token để xác thực phiên làm việc.
            
-  Hệ thống xác nhận và bắt đầu nhận vị trí GPS từ thiết bị.
            
-  Ứng dụng gửi sự kiện \texttt{user:move} liên tục với tọa độ GPS hiện tại.
            
-  Bản đồ hiển thị avatar của sinh viên tại vị trí thực tế trên campus.
            
-  Hệ thống tự động gửi lại \texttt{user:state\_update} khi có người dùng khác ở gần, ứng dụng hiển thị avatar của họ trên bản đồ.
            
-  Sinh viên có thể chạm vào avatar của người dùng khác để xem hồ sơ.
          \\
        \hline
        **Luồng sự kiện phát sinh** &
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  *F1 --- GPS không khả dụng:* Ứng dụng thông báo yêu cầu bật GPS, vẫn hiển thị bản đồ nhưng không có vị trí cá nhân.
            
-  *F2 --- Mất kết nối WebSocket:* Ứng dụng tự động thử kết nối lại sau 3 giây; hiển thị trạng thái ``Đang kết nối lại\ldots'' trên màn hình bản đồ.
          \\
        \hline
    \end{tabular}
\end{table}

### Đặc tả use case: Tạo và xem hành trình cá nhân

\begin{table}[H]
    \centering
    \caption{Đặc tả use case: Xem hành trình di chuyển cá nhân}
    
    \renewcommand{\arraystretch}{1.3}
    \begin{tabular}{|p{3.5cm}|p{11cm}|}
        \hline
        **Tên use case** & **Xem hành trình di chuyển trong campus được tự động tổng hợp** \\
        \hline
        **Mã use case** & UC-03 \\
        \hline
        **Tác nhân** & Sinh viên (đã đăng nhập, đã bật GPS trong ngày) \\
        \hline
        **Mô tả ngắn gọn** & Hệ thống tự động theo dõi GPS và tổng hợp lại thành chuyến hành trình trong ngày của sinh viên. \\
        \hline
        **Tiền điều kiện** & Sinh viên đã di chuyển trong campus ít nhất một khoảng thời gian đủ để hệ thống ghi nhận vị trí. \\
        \hline
        **Hậu điều kiện** & Hành trình draft được tạo tự động, hiển thị trên bản đồ với các điểm dừng được đặt tên theo tòa nhà. \\
        \hline
        **Luồng sự kiện chính** & 
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  Sinh viên chọn chức năng ``Hành trình hôm nay'' từ màn hình hồ sơ.
            
-  Ứng dụng gọi API để yêu cầu server tổng hợp hành trình từ dữ liệu GPS trong ngày.
            
-  Server phân tích dữ liệu vị trí: phát hiện các điểm dừng chân, loại bỏ nhiễu GPS, và tra cứu tên tòa nhà gần nhất.
            
-  Server trả về hành trình draft gồm danh sách điểm dừng (tên tòa nhà, thời gian) và đường đi kết nối.
            
-  Ứng dụng hiển thị hành trình trên bản đồ, vẽ đường đi và đánh dấu từng điểm dừng bằng icon số thứ tự.
            
-  Sinh viên có thể chỉnh sửa tên điểm dừng, thêm ảnh, thêm ghi chú.
            
-  Sinh viên nhấn ``Xuất bản'' để chia sẻ hành trình lên mạng xã hội.
          \\
        \hline
        **Luồng sự kiện phát sinh** &
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  *F1 --- Không có dữ liệu GPS:* Hệ thống thông báo chưa có dữ liệu vị trí đủ để tổng hợp hành trình và hướng dẫn bật tính năng chia sẻ vị trí.
            
-  *F2 --- Hành trình quá ngắn:* Nếu sinh viên chỉ ở một chỗ trong ngày, hệ thống vẫn tạo hành trình với một điểm duy nhất và thông báo tương ứng.
          \\
        \hline
    \end{tabular}
\end{table}

### Đặc tả use case: Tham gia và xem sự kiện campus

\begin{table}[H]
    \centering
    \caption{Đặc tả use case: Xem và tham gia sự kiện campus}
    
    \renewcommand{\arraystretch}{1.3}
    \begin{tabular}{|p{3.5cm}|p{11cm}|}
        \hline
        **Tên use case** & **Xem và tham gia sự kiện campus** \\
        \hline
        **Mã use case** & UC-04 \\
        \hline
        **Tác nhân** & Sinh viên (đã đăng nhập) \\
        \hline
        **Mô tả ngắn gọn** & Cho phép sinh viên theo dõi thông tin và đăng ký tham gia các sự kiện đang diễn ra trong khuôn viên. \\
        \hline
        **Tiền điều kiện** & Có ít nhất một sự kiện đang diễn ra hoặc sắp diễn ra trong hệ thống. \\
        \hline
        **Hậu điều kiện** & Sinh viên đăng ký tham gia sự kiện và nhận thông báo nhắc nhở khi sự kiện bắt đầu. \\
        \hline
        **Luồng sự kiện chính** & 
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  Sinh viên thấy biểu tượng sự kiện trên bản đồ campus hoặc trong danh sách sự kiện.
            
-  Sinh viên nhấp vào biểu tượng sự kiện để xem chi tiết: tên, mô tả, thời gian, địa điểm, số người tham gia.
            
-  Sinh viên nhấn ``Tham gia'' để đăng ký; hệ thống ghi nhận và cập nhật số lượng người tham dự.
            
-  Nếu sự kiện có live stream, sinh viên nhấn ``Xem stream'' để vào phòng LiveKit và xem hoặc phát trực tiếp.
          \\
        \hline
        **Luồng sự kiện phát sinh** &
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  *F1 --- Sự kiện đã bị hủy/kết thúc:* Nếu sự kiện vừa bị người tổ chức hủy hoặc đã kết thúc, hệ thống hiển thị thông báo "Sự kiện không còn khả dụng" và gỡ khỏi bản đồ.
          \\
        \hline
    \end{tabular}
\end{table}

### Đặc tả use case: Khởi chạy mô phỏng dự đoán mật độ

\begin{table}[H]
    \centering
    \caption{Đặc tả use case: Khởi chạy mô phỏng dự đoán mật độ}
    
    \renewcommand{\arraystretch}{1.3}
    \begin{tabular}{|p{3.5cm}|p{11cm}|}
        \hline
        **Tên use case** & **Khởi chạy mô phỏng kịch bản dự đoán mật độ** \\
        \hline
        **Mã use case** & UC-05 \\
        \hline
        **Tác nhân** & Quản trị viên \\
        \hline
        **Mô tả ngắn gọn** & Cho phép quản trị viên xem bản đồ dự báo mật độ sinh viên trong khuôn viên dựa trên các kịch bản thời gian tương lai. \\
        \hline
        **Tiền điều kiện** & Quản trị viên đã đăng nhập vào hệ thống. Hệ thống đã thu thập và có sẵn mô hình dữ liệu lịch sử về vị trí sinh viên. \\
        \hline
        **Hậu điều kiện** & Hệ thống tính toán và hiển thị bản đồ mật độ dự kiến cho thời điểm được chọn. \\
        \hline
        **Luồng sự kiện chính** & 
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  Quản trị viên chọn ``Bản đồ mật độ'' ở thanh menu, và chuyển sang chế độ ``Giả Lập''.
            
-  Hệ thống hiển thị giao diện bản đồ campus cùng công cụ chọn thời gian, thời tiết, đóng mở building.
            
-  Quản trị viên thiết lập kịch bản và nhấn ``Chạy mô phỏng''.
            
-  Hệ thống gọi dịch vụ mô phỏng, phân tích dữ liệu lịch sử và các luồng di chuyển để tính toán mật độ.
            
-  Hệ thống trả về kết quả và hiển thị trực tiếp bằng biểu đồ nhiệt trên bản đồ.
          \\
        \hline
        **Luồng sự kiện phát sinh** &
        
        [leftmargin=*, topsep=0pt, partopsep=0pt, parsep=0pt, itemsep=0pt]
            
-  *F1 --- Không đủ dữ liệu:* Nếu hệ thống chưa có đủ mẫu dữ liệu để chạy mô phỏng cho thời điểm đó, hệ thống sẽ cảnh báo lỗi và ngừng xử lý.
            
-  *F2 --- Lỗi kết nối máy chủ mô phỏng:* Hệ thống thông báo lỗi kết nối và gợi ý quản trị viên thử lại sau.
          \\
        \hline
    \end{tabular}
\end{table}

## Yêu cầu phi chức năng

Dựa trên đặc thù của một hệ thống Digital Twin quản lý dữ liệu không gian thời gian thực, hệ thống Hust Simulator được thiết kế nhằm đáp ứng các yêu cầu phi chức năng khắt khe, qua đó khẳng định sự mạnh mẽ và ổn định của nền tảng khi vận hành thực tế ở quy mô lớn.

**Yêu cầu về hiệu năng:** Hệ thống sở hữu năng lực xử lý vượt trội, cho phép tiếp nhận và đồng bộ mượt mà tải trọng từ hàng ngàn luồng dữ liệu thời gian thực. Hệ thống phải đảm bảo khả năng đáp ứng cho hơn 5.000 người dùng đồng thời, với độ trễ đồng bộ trạng thái được duy trì ở mức dưới 30ms, đảm bảo mọi di chuyển ngoài đời thực đều được phản ánh gần như tức thời trên không gian số.

**Yêu cầu về bảo mật:** Hệ thống đặt tính toàn vẹn dữ liệu và quyền riêng tư của sinh viên lên hàng đầu. Toàn bộ các phiên giao tiếp và kết nối thời gian thực đều được xác thực chặt chẽ, đảm bảo chỉ người dùng hợp lệ mới có thể tương tác.

**Yêu cầu về khả năng mở rộng:** Điểm sáng của hệ thống nằm ở kiến trúc Microservices kết hợp cùng kỹ thuật Spatial Sharding. Việc chia nhỏ thành các dịch vụ độc lập giúp hệ thống vô cùng linh hoạt trong việc tích hợp hoặc bổ sung các service nghiệp vụ mới. Đồng thời, nhờ cơ chế Spatial Sharding, nền tảng rất dễ dàng mở rộng theo chiều ngang để xử lý lượng tải ngày càng tăng mà không gặp phải giới hạn về điểm nghẽn cổ chai.

**Yêu cầu về khả năng sử dụng:** Thay vì chỉ phục vụ một bài toán duy nhất, toàn bộ hệ thống được đóng gói bài bản và có khả năng tích hợp linh hoạt với các mô hình Digital Twin ở nhiều lĩnh vực khác nhau (như giao thông thông minh, quản lý sự kiện). Việc tích hợp và tái sử dụng nền tảng này diễn ra đơn giản, bởi toàn bộ các nghiệp vụ phức tạp đã được chia nhỏ và đóng gói độc lập, cung cấp sẵn bộ khung vững chắc cho các ứng dụng phân tích phía trên khai thác.



<!-- END OF 2_Khao_sat.tex -->
---


<!-- START OF 3_Cong_nghe.tex (Chương 3: Nền tảng lý thuyết và công nghệ) -->



Chương này trình bày về các quyết định lựa chọn công nghệ cốt lõi phục vụ việc xây dựng nền tảng Hust Simulator.

## Kiến trúc Microservices

Trước khi đi vào chi tiết hệ thống, chúng ta cần làm rõ lý do lựa chọn kiến trúc Microservices [newman2015microservices] --- vốn phức tạp hơn đáng kể so với kiến trúc Monolith truyền thống. Ngay từ giai đoạn hình thành ý tưởng, hệ thống đã được hình dung là phải xử lý nhiều bài toán con khác nhau, mỗi bài toán đòi hỏi những kỹ thuật và công nghệ riêng biệt. Nếu triển khai theo hướng Monolith, hệ thống sẽ gặp phải ba vấn đề chính sau:

Đầu tiên chính là về khả năng mở rộng. Mặc dù phạm vi đồ án hiện tại được giới hạn trong khuôn viên Đại học Bách Khoa, nhưng mục tiêu dài hạn là có thể mở rộng bộ khung này sang các domain khác, chẳng hạn như giao thông thông minh. Với kiến trúc Monolith, việc mở rộng như vậy gần như đồng nghĩa với việc viết lại toàn bộ hệ thống từ đầu. Ngược lại, khi các service được tách biệt rõ ràng, hệ thống sẽ vận hành như một bộ khung lắp ráp --- cho phép lựa chọn và kết hợp các thành phần phù hợp để xây dựng nên hệ thống mới mà không cần xây lại từ con số không.

Thứ hai là về khả năng cô lập lỗi. Trong kiến trúc Monolith, một lỗi xảy ra ở bất kỳ thành phần nào cũng có nguy cơ kéo theo sự sụp đổ của toàn bộ hệ thống. Với Microservices, lỗi tại một service sẽ được giới hạn phạm vi ảnh hưởng, không làm gián đoạn hoạt động của các service còn lại.

Và cuối cùng chính là về tính linh hoạt trong lựa chọn công nghệ. Mỗi bài toán con trong hệ thống có đặc thù kỹ thuật khác nhau, phù hợp với những ngôn ngữ, framework hoặc công cụ khác nhau. Kiến trúc Microservices cho phép từng service được phát triển độc lập với stack công nghệ riêng, thay vì bị ràng buộc bởi một nền tảng công nghệ duy nhất như trong Monolith.

Từ ba lý do trên, Microservices được lựa chọn làm kiến trúc nền tảng cho hệ thống, dù đòi hỏi chi phí thiết kế và vận hành phức tạp hơn so với Monolith. Dưới đây là các service đã được phát triển nhằm giải quyết từng bài toán cụ thể:

## Ngôn ngữ và Framework phát triển

Việc áp dụng Microservices cho phép mỗi service được xây dựng trên một stack công nghệ phù hợp nhất với đặc thù xử lý riêng biệt.

### Dịch vụ nghiệp vụ cốt lõi: Java và Spring Boot

Với các dịch vụ chịu trách nhiệm xử lý các nghiệp vụ cốt lõi của hệ thống, đòi hỏi khả năng quản lý giao dịch, đảm bảo tính toàn vẹn dữ liệu và tổ chức mã nguồn theo miền nghiệp vụ, Spring Boot [walls2022spring] được lựa chọn làm nền tảng phát triển cho nhóm dịch vụ này.

![Dependency Injection](Hinhve/DependencyInjection.png)
*Hình: Dependency Injection*

Spring Boot cung cấp cơ chế Dependency Injection giúp giảm sự phụ thuộc giữa các thành phần và hỗ trợ phát triển theo kiến trúc phân lớp. Đồng thời, framework này còn cung cấp Transaction Management, khả năng giúp đảm bảo tính nhất quán của dữ liệu khi thao tác trên nhiều bảng hoặc nhiều thành phần liên quan. Có thể quan sát Hình [fig:di] và Hình [fig:transaction] để hiểu rõ hơn về cơ chế hoạt động của hai tính năng quan trọng này. Bên cạnh đó, hệ sinh thái Java với JVM, ORM và các công cụ hỗ trợ Domain-Driven Design [evans2003ddd] được đánh giá phù hợp hơn đối với các nghiệp vụ phức tạp. Do đó, Spring Boot được lựa chọn cho các dịch vụ nghiệp vụ cốt lõi của hệ thống.

### Dịch vụ thời gian thực: Node.js và NestJS

Khác với các dịch vụ nghiệp vụ, cụm service thời gian thực phải duy trì đồng thời một số lượng lớn kết nối WebSocket và xử lý liên tục các bản cập nhật vị trí từ người dùng. Đây là bài toán I/O-bound điển hình, trong đó chi phí chủ yếu nằm ở việc quản lý kết nối và chờ dữ liệu từ mạng thay vì thực hiện các phép tính phức tạp.

Vì vậy, Node.js [tilkov2010nodejs] được lựa chọn nhờ cơ chế Event Loop với Non-blocking I/O, cho phép xử lý hiệu quả số lượng lớn kết nối đồng thời với chi phí tài nguyên thấp.

![Cơ chế hoạt động của Event Loop trong Node.js](Hinhve/EventLoop.png)
*Hình: Cơ chế hoạt động của Event Loop trong Node.js*

Trong kiến trúc máy chủ đa luồng truyền thống, mỗi kết nối người dùng mới thường yêu cầu cấp phát một luồng xử lý riêng biệt. Khi số lượng người dùng đồng thời đạt đến hàng nghìn, bộ nhớ sẽ nhanh chóng cạn kiệt và chi phí chuyển ngữ cảnh giữa các luồng làm sụt giảm hiệu năng nghiêm trọng. 

Ngược lại, Node.js vận hành trên một luồng chính duy nhất kết hợp với cơ chế Event Loop. Khi tiếp nhận một tác vụ I/O chậm (chẳng hạn như đọc ghi database hay nhận luồng WebSocket), luồng chính sẽ không bị chặn để chờ tác vụ hoàn tất, mà lập tức đẩy nó xuống cho hệ thống nền xử lý và tiếp tục nhận các yêu cầu khác. Khi thao tác I/O hoàn thành, kết quả cùng hàm callback tương ứng sẽ được đẩy vào Event Queue. Event Loop sẽ liên tục vòng lặp, nhặt các sự kiện đã hoàn thành từ hàng đợi và đưa lên luồng chính để phản hồi cho người dùng (Hình [fig:event_loop]). 

Nhờ kiến trúc này, Node.js có thể duy trì hàng nghìn kết nối WebSocket đồng thời mà không bị nghẽn luồng xử lý chính. Đặc điểm này hoàn toàn phù hợp với bài toán I/O-bound của Hust Simulator, nơi mạng lưới người dùng liên tục trao đổi các gói dữ liệu vị trí nhỏ nhưng với tần suất cực kỳ cao.

Trên nền tảng Node.js, NestJS [nestjsdocs] được lựa chọn nhờ kiến trúc module hóa và khả năng tích hợp tốt với TypeScript, giúp hệ thống thời gian thực dễ bảo trì và mở rộng. Do đó, Node.js và NestJS được sử dụng cho các dịch vụ thời gian thực.

### Dịch vụ dự báo: Python và Scikit-Learn

Với dịch vụ chịu trách nhiệm khai thác dữ liệu lịch sử để dự báo xu hướng phân bố người dùng trong tương lai, nó đòi hỏi khả năng xử lý dữ liệu và xây dựng các mô hình dự báo trên dữ liệu không gian – thời gian. Vì vậy, Python được lựa chọn nhờ hệ sinh thái khoa học dữ liệu phong phú, hỗ trợ toàn bộ quy trình từ xử lý dữ liệu, huấn luyện mô hình đến triển khai. Trên nền tảng đó, Scikit-Learn được sử dụng để xây dựng và huấn luyện mô hình học máy nhờ tính ổn định, dễ triển khai và sự phổ biến rộng rãi. Các công cụ và thư viện hiện đại trong lĩnh vực học máy đều được hỗ trợ rất tốt trên Python, giúp thuận lợi cho việc nghiên cứu và hiện thực hóa vào hệ thống.

Do đó, Python kết hợp với thư viện Scikit-Learn được lựa chọn cho Prediction Service nhằm hỗ trợ hiệu quả quá trình xây dựng và triển khai mô hình dự báo.

## Cơ sở dữ liệu và giao thức trao đổi thông điệp

### Hệ quản trị cơ sở dữ liệu: PostgreSQL và Redis

Dữ liệu trong Hust Simulator bao gồm hai nhóm có đặc tính khác nhau: dữ liệu nghiệp vụ tĩnh đòi hỏi tính nhất quán cao, và dữ liệu trạng thái thời gian thực đòi hỏi tốc độ truy xuất lớn. Do đó, hệ thống áp dụng mô hình **Polyglot Persistence**, kết hợp giữa PostgreSQL và Redis.

Về dữ liệu nghiệp vụ tĩnh, PostgreSQL [postgresql] được lựa chọn làm kho lưu trữ chính nhờ khả năng quản lý dữ liệu quan hệ chặt chẽ, đảm bảo tính nhất quán cao cho các giao dịch phức tạp, cùng sức mạnh xử lý dữ liệu không gian thông qua phần mở rộng PostGIS [postgis_in_action].

Về dữ liệu thời gian thực, Redis được sử dụng như một in-memory database để lưu trữ các trạng thái cập nhật liên tục, chẳng hạn tọa độ thời gian thực và trạng thái trực tuyến của sinh viên. Tốc độ truy xuất vượt trội của Redis giúp hệ thống xử lý mượt mà hàng nghìn sự kiện mỗi giây, đồng thời giảm tải đáng kể các tác vụ ghi liên tục lên PostgreSQL.

Sự kết hợp giữa sức mạnh quản lý quan hệ của PostgreSQL và tốc độ của Redis giúp nền tảng vừa đảm bảo tính toàn vẹn dữ liệu dài hạn, vừa đáp ứng tốt yêu cầu xử lý thời gian thực.

![Mô hình lưu trữ hỗn hợp Polyglot Persistence](Hinhve/polyglot_persistence.png)
*Hình: Mô hình lưu trữ hỗn hợp Polyglot Persistence*

### Giao thức trao đổi thông điệp: REST, gRPC và RabbitMQ

Hệ thống sử dụng kết hợp REST, gRPC và RabbitMQ để tận dụng ưu điểm của từng cơ chế. 
REST API được sử dụng cho giao tiếp giữa client và hệ thống. Các chức năng như xác thực, quản lý người dùng hay truy vấn dữ liệu được triển khai thông qua các endpoint RESTful, bảo đảm tính đơn giản và khả năng tương thích rộng rãi. 
Đối với giao tiếp nội bộ giữa các service yêu cầu phản hồi nhanh, hệ thống sử dụng gRPC [grpc]. Nhờ cơ chế mã hóa Protocol Buffers và các tối ưu của HTTP/2, gRPC giúp giảm chi phí truyền dữ liệu và phù hợp với các tác vụ được thực hiện thường xuyên với tần suất cao. 
Bên cạnh đó, RabbitMQ [rabbitmq] được sử dụng cho các luồng xử lý bất đồng bộ. Các sự kiện phát sinh từ một service sẽ được công bố lên Message Broker để các service khác tiếp nhận và xử lý độc lập. Cách tiếp cận này giúp giảm sự phụ thuộc giữa các thành phần, tăng khả năng chịu lỗi và hỗ trợ mở rộng hệ thống.

![Các giao thức và mô hình giao tiếp trong hệ thống](Hinhve/protocol.png)
*Hình: Các giao thức và mô hình giao tiếp trong hệ thống*

Sự kết hợp này cho phép hệ thống đáp ứng đồng thời các yêu cầu về hiệu năng, khả năng mở rộng và tính ổn định.

## Mở rộng khả năng xử lý đồng thời bằng kiến trúc Spatial Sharding

Khi nhắc đến Digital Twin, kỹ thuật không thể thiếu chính là **Spatial Sharding** [eldawy2016bigspatial]. Về cơ bản, đây là kỹ thuật phân chia không gian thành các vùng độc lập, mỗi vùng được giao cho một node xử lý. Đây là kỹ thuật tất yếu, đóng vai trò tiền đề giúp hệ thống có khả năng mở rộng theo chiều ngang, khi phạm vi xử lý không còn giới hạn trong khuôn viên Đại học Bách Khoa mà mở rộng ra các khu vực khác.

Dựa trên nguyên lý đó, hệ thống đã hiện thực hóa Spatial Sharding thông qua việc phân chia trách nhiệm quản lý trạng thái theo ba khu vực khác nhau. Mỗi phân vùng chịu trách nhiệm quản lý trạng thái của các người dùng đang hiện diện trong khu vực tương ứng, qua đó giới hạn phạm vi xử lý của từng node và tránh việc toàn bộ trạng thái phải tập trung trên một máy chủ duy nhất. 

Trên mỗi phân vùng, **Interest Matcher Service** vận hành dựa trên cơ chế **Publish-Subscribe**. Lúc này, service đóng vai trò như một *Broker* trung tâm chuyên trách điều phối dữ liệu không gian. Cụ thể, như minh họa tại Hình [fig:interest_matcher], khi một người dùng di chuyển, thiết bị của họ sẽ đóng vai trò là một *Publisher*, liên tục gửi tọa độ mới nhất lên Broker phụ trách khu vực đó. Ở chiều ngược lại, thay vì phát sóng dữ liệu này cho toàn bộ mọi người, Broker sẽ đóng vai trò phân phối trạng thái mới này chỉ đến những người dùng khác đang có mặt trong cùng khu vực và đã đăng ký theo dõi luồng dữ liệu (đóng vai trò là *Subscribers*).

![Interest Matcher Broker](Hinhve/InterestMatcher.png)
*Hình: Interest Matcher Broker*

Kiến trúc Spatial Sharding tạo nền tảng cho khả năng mở rộng theo chiều ngang của hệ thống. Do trạng thái được phân tách theo các khu vực độc lập, các *Interest Matcher Broker* có thể được triển khai trên nhiều máy chủ phân tán, trong đó mỗi máy chủ đảm nhiệm một phần không gian riêng biệt. Nhờ đó, tải xử lý được phân bố đều giữa các nút, giảm nguy cơ hình thành điểm nghẽn và cho phép hệ thống duy trì khả năng đồng bộ thời gian thực cho số lượng lớn người dùng đồng thời.

## Cơ sở toán học của mô hình dự báo hành vi di chuyển

Để dự báo quỹ đạo di chuyển của sinh viên, hệ thống sử dụng ba công cụ toán học cốt lõi bao gồm: chuỗi Markov bậc một, ước lượng mật độ hạt nhân, và thuật toán tối ưu hóa SLSQP.

### Chuỗi Markov bậc một

Chuỗi Markov mô tả quá trình chuyển đổi của hệ thống qua các trạng thái rời rạc thuộc tập $S = \{s_1, \dots, s_k\}$. Mô hình này thỏa mãn tính chất Markov: xác suất chuyển sang trạng thái tiếp theo chỉ phụ thuộc vào trạng thái hiện tại:

$$
    P(X_{t+1} = s_j \mid X_t = s_i, \dots, X_0 = s_0) = P(X_{t+1} = s_j \mid X_t = s_i)
$$

Xác suất này được biểu diễn qua ma trận chuyển tiếp $T \in \mathbb{R}^{k \times k}$ với các phần tử $T_{ij} = P(X_{t+1} = s_j \mid X_t = s_i)$ thỏa mãn $T_{ij} \ge 0$ và $\sum_{j=1}^{k} T_{ij} = 1$. Trong đề tài, chuỗi Markov được sử dụng để ước lượng xác suất chuyển tiếp giữa các địa điểm dừng (POI) liên tiếp của người dùng.

### Ước lượng mật độ hạt nhân Gaussian

Ước lượng mật độ hạt nhân (KDE) là phương pháp phi tham số để ước lượng hàm mật độ xác suất của biến ngẫu nhiên liên tục. Với mẫu quan sát $\{x_1, \dots, x_N\}$, hàm mật độ tại $x$ được xác định bởi:

$$
    f(x) = \frac{1}{N \cdot h} \sum_{i=1}^{N} K\left( \frac{x - x_i}{h} \right)
$$

Trong đó, hàm nhân Gauss $K(u) = \frac{1}{\sqrt{2\pi}} \exp\left( -\frac{u^2}{2} \right)$ được sử dụng để làm trơn phân phối. Băng thông $h > 0$ quyết định mức độ làm trơn phân phối: $h$ quá nhỏ gây nhiễu và quá khớp, ngược lại $h$ quá lớn làm mất các đặc trưng cục bộ. Trong đề tài, KDE được dùng để mô hình hóa phân bố thời gian xuất hiện của người dùng tại các địa điểm.

### Tối ưu hóa có ràng buộc bằng thuật toán SLSQP

SLSQP là phương pháp tối ưu hóa số học để giải bài toán cực tiểu hóa có ràng buộc:

$$
    \min_{\mathbf{w}} F(\mathbf{w}) \quad \text{thỏa mãn} \quad g_i(\mathbf{w}) \ge 0, \quad h_j(\mathbf{w}) = 0
$$

Trong đó, $F(\mathbf{w})$ là hàm mục tiêu; $g_i(\mathbf{w})$ và $h_j(\mathbf{w})$ lần lượt là các hàm ràng buộc bất đẳng thức và đẳng thức. SLSQP giải bài toán bằng cách lặp lại việc xấp xỉ hàm mục tiêu và ràng buộc thành bài toán quy hoạch bậc hai (Quadratic Programming), rồi cập nhật nghiệm đến khi hội tụ. Trong đề tài, SLSQP được sử dụng để tối ưu hóa bộ trọng số của mô hình kết hợp nhằm cực đại hóa độ chính xác dự báo trên tập validation.

## Tổng hợp và dự báo phân bố mật độ toàn cục

Nhưng khi phân tích ở góc độ quản trị, chúng ta không thể chỉ "soi" vào dữ liệu của từng cá nhân để đưa ra kết luận. Nếu hệ thống chỉ hiển thị hàng nghìn hướng di chuyển rời rạc của từng sinh viên, quản trị viên sẽ rơi vào tình trạng quá tải thông tin và không thể đánh giá được trạng thái chung của toàn khuôn viên.

![Pipeline bản đồ phân bố mật độ người dùng](Hinhve/PipelineDuDoan.png)
*Hình: Pipeline bản đồ phân bố mật độ người dùng*

Để giải quyết giới hạn này, bước thứ hai của hệ thống là áp dụng các phương pháp tổng hợp để **chuyển các hành vi cá nhân thành một bức tranh tổng thể**. Toàn bộ kết quả dự đoán đích đến từ mô hình dự đoán cá nhân sẽ được gom nhóm và ánh xạ lên mạng lưới không gian để tạo thành các **bản đồ mật độ dự báo (Predictive Heatmap)**. 

Ý tưởng cơ bản của Predictive Heatmap là tổng hợp kết quả dự báo của từng cá nhân lên cùng một không gian bản đồ. Mỗi người dùng đóng góp một đơn vị trọng số vào bản đồ, sau đó trọng số này được phân bổ vào các tòa nhà và mạng lưới giao thông nội khu để tạo ra phân bố mật độ liên tục thay vì các điểm rời rạc:

    
-  **Phân bổ tại tòa nhà:** Khi một sinh viên được dự báo đến một tòa nhà, trọng số không dồn vào tâm mà được phân tán lên các ô lưới bên trong ranh giới tòa nhà. Mật độ tuân theo **phân phối chuẩn** tính từ tâm, giúp tập trung phần lớn đám đông ở khu vực cốt lõi và giảm dần về phía rìa. Toàn bộ trọng số được giới hạn nghiêm ngặt bên trong không gian tòa nhà, đảm bảo hình ảnh heatmap phản ánh đúng hình dạng thực tế và không tràn ra ngoài.
    
    
-  **Lưu thông trên đường nội khu:** Hệ thống trích một phần trọng số dự báo để dàn trải lên hệ thống giao thông. Mật độ tại từng đoạn đường tỉ lệ thuận với mức độ đông đúc của các tòa nhà lân cận, giúp khu vực trước các giảng đường lớn hiển thị lưu lượng người qua lại cao hơn. Để mô phỏng chân thực từng tốp sinh viên di chuyển thay vì một dải nhiệt tĩnh, hệ thống áp dụng cơ chế nhiễu không gian **Flocking Noise**. Hàm Harmonic Flocking liên tục dịch chuyển các vùng sáng tối theo biến thời gian, tái tạo thành công hình ảnh các dòng người đang đi lại ngắt quãng trên đường.

Bên cạnh thói quen di chuyển cá nhân, mô hình tổng hợp còn liên tục được tinh chỉnh bởi các dữ liệu ngữ cảnh toàn cục để phục vụ cho các bài toán phân tích giả định:

    
-  **Pha thời gian:** Ánh xạ thời điểm dự báo thành các pha hoạt động đặc trưng của trường. Mỗi pha quyết định tỷ lệ người đang lưu thông trên đường và hệ số nhân lưu lượng tổng thể.
    
-  **Sự kiện:** Các sự kiện lớn làm thay đổi cục diện phân bố. Hệ thống tự động điều hướng trọng số dự báo của đám đông về phía sự kiện để hiển thị rõ sự kiện như những "điểm nóng" ngay cả khi dữ liệu thói quen cá nhân chưa kịp cập nhật.
    
-  **Trạng thái khu vực:** Trạng thái đóng/mở của các tòa nhà giới hạn không gian di chuyển. Khi một khu vực bị đóng, hệ thống tự động loại trừ nó khỏi danh sách điểm đến và phân bổ lại trọng số dự báo sang các khu vực lân cận thay thế.

Chỉ thông qua phương pháp tổng hợp này, những dữ liệu thô và dự đoán cá nhân rời rạc mới thực sự biến thành tri thức quản trị, cho phép nhà quản lý dễ dàng nhận diện các điểm nóng nguy cơ quá tải và chủ động điều phối hoạt động.



<!-- END OF 3_Cong_nghe.tex -->
---


<!-- START OF 4_Phan_tich_thiet_ke.tex (Chương 4: Phân tích thiết kế) -->



	## Thiết kế kiến trúc

	### Lựa chọn kiến trúc phần mềm

	Ở cấp độ toàn hệ thống được thiết kế tổ chức theo mô hình kiến trúc
	ba tầng, bao gồm **Tầng Trình diễn (Presentation Layer)**, **Tầng
	Nghiệp vụ (Application Layer)** và **Tầng Dữ liệu (Data Layer)**.

	Kiến trúc này giúp phân tách các thành phần của hệ thống thành các lớp chức năng độc lập, giảm sự phụ thuộc trực tiếp giữa giao diện, logic xử lý và dữ liệu lưu trữ.

	Việc áp dụng kiến trúc ba tầng mang lại nhiều lợi ích cho hệ thống. Trước hết, cấu trúc phân lớp giúp mã nguồn được tổ chức rõ ràng, thuận lợi cho quá trình phát triển, kiểm thử và bảo trì. Bên cạnh đó, mỗi tầng có thể được mở rộng hoặc nâng cấp tương đối độc lập, giúp hệ thống thích ứng tốt với sự gia tăng số lượng người dùng và khối lượng dữ liệu trong tương lai. Mô hình này cũng tạo điều kiện thuận lợi cho việc triển khai theo hướng Microservices và điện toán đám mây, phù hợp với yêu cầu xử lý thời gian thực của Hust Simulator.

	
![Mô hình kiến trúc ba tầng của hệ thống](Hinhve/3-tier.png)
*Hình: Mô hình kiến trúc ba tầng của hệ thống*

	Mô hình ba tầng được áp dụng như sau:

	
		
-  **Tầng Trình diễn (Presentation Layer):** Bao gồm ứng dụng phía
			người dùng cuối và ứng dụng quản trị dành cho quản trị viên. Tầng này chịu 
			trách nhiệm hiển thị giao diện và tiếp nhận các thao tác của người dùng.

		
-  **Tầng Nghiệp vụ (Application Layer):** Là nơi triển khai kiến
			trúc Microservices nhằm xử lý logic cốt lõi và điều phối dữ liệu giữa các
			tầng của hệ thống.

		
-  **Tầng Dữ liệu (Data Layer):** Chịu trách nhiệm lưu trữ, quản lý và
			cung cấp dữ liệu cho toàn bộ hệ thống. Tầng này kết hợp nhiều giải pháp
			lưu trữ chuyên biệt, bao gồm PostgreSQL cho dữ liệu quan hệ, Redis cho các
			tác vụ bộ nhớ đệm và lưu trữ tạm thời, cùng với Firebase Storage để lưu trữ
			các tệp tin.
	

	Đi sâu vào bên trong từng service, mã nguồn được thiết kế tổ chức theo kiến trúc
	phân lớp **Controller-Service-Repository**.

	
![Kiến trúc phân lớp Controller -- Service -- Repository bên trong các
		service](
			Hinhve/Controller-Service-Repository.png
		)
*Hình: Kiến trúc phân lớp Controller -- Service -- Repository bên trong các
		service*

	Chi tiết ba lớp chính:

	
		
-  **Lớp Controller (Điều khiển):** Đóng vai trò là cửa ngõ giao tiếp của từng service. Lớp này chịu trách nhiệm tiếp nhận các yêu cầu từ bên ngoài, tiến hành xác thực và kiểm tra tính hợp lệ của dữ liệu đầu vào. Sau khi xác định dữ liệu an toàn, Controller sẽ định tuyến và chuyển tiếp luồng xử lý xuống lớp Service tương ứng, đồng thời định dạng dữ liệu kết quả để trả về cho client.

		
-  **Lớp Service (Nghiệp vụ):** Là trung tâm của ứng dụng, nơi tập trung triển khai mọi quy tắc và logic nghiệp vụ cốt lõi. Lớp này nhận dữ liệu đã được làm sạch từ Controller, thực hiện các chuỗi tính toán, kiểm tra điều kiện và điều phối các thành phần. Việc cô lập hoàn toàn logic tại lớp Service giúp mã nguồn dễ dàng tái sử dụng mà không bị phụ thuộc vào giao thức mạng hay cơ sở dữ liệu.

		
-  **Lớp Repository (Truy xuất dữ liệu):** Đảm nhiệm chuyên biệt việc giao tiếp với các hệ thống lưu trữ. Lớp này cung cấp các phương thức giao tiếp trừu tượng để thực hiện thao tác thêm, sửa, xóa và truy vấn dữ liệu. Nhờ có Repository, lớp Service không cần trực tiếp xử lý các câu lệnh truy vấn cơ sở dữ liệu, từ đó đảm bảo tính linh hoạt cao nếu hệ thống cần thay đổi hoặc nâng cấp công nghệ lưu trữ.
	

	Sự kết hợp giữa kiến trúc ba tầng ở cấp độ hệ thống và kiến trúc phân lớp bên
	trong từng service giúp Hust Simulator phân tách rõ ràng trách nhiệm của các
	thành phần, giảm mức độ phụ thuộc giữa chúng, từ đó nâng cao khả năng bảo trì, mở
	rộng và phát triển hệ thống trong tương lai.

	### Thiết kế tổng quan

	Dựa trên tư tưởng thiết kế, kiến trúc Backend của hệ thống được phân chia thành
	bảy gói chính, trong đó mỗi gói đại diện cho một miền nghiệp vụ độc lập. Việc phân
	tách này nhằm đảm bảo tính độc lập giữa các chức năng, giảm sự phụ thuộc chéo
	và tạo điều kiện thuận lợi cho việc bảo trì cũng như mở rộng hệ thống trong
	tương lai.

	
![Biểu đồ gói tổng quan của hệ thống](Hinhve/package_diagram.png)
*Hình: Biểu đồ gói tổng quan của hệ thống*

	Kiến trúc hệ thống được tổ chức thành ba tầng logic, trong đó các quan hệ phụ
	thuộc giữa các gói chỉ được phép diễn ra theo chiều từ tầng trên xuống tầng
	dưới.

	
		
-  **Tầng 1 -- Foundation Layer:**

			Đây là tầng nền tảng của hệ thống, cung cấp các dịch vụ cơ bản và dữ liệu
			dùng chung cho các miền nghiệp vụ ở tầng phía trên.

			
				
-  **Gói *auth**:* Chịu trách nhiệm quản lý định danh
					người dùng, xác thực và phân quyền truy cập. Gói này hoạt động độc lập
					và không phụ thuộc vào bất kỳ thành phần nội bộ nào khác.

				
-  **Gói *infrastructure**:* Quản lý các thực thể không
					gian tĩnh như tòa nhà, phòng học và các điểm điều hướng. Gói này đóng
					vai trò cung cấp dữ liệu bản đồ và tọa độ phục vụ cho các chức năng
					khác của hệ thống.
			

		
-  **Tầng 2 -- Core Domains Layer:**

			Tầng này bao gồm các miền nghiệp vụ chính của hệ thống và được xây dựng dựa
			trên các dịch vụ nền tảng ở tầng dưới.

			
				
-  **Gói *social**:* Cung cấp các chức năng kết bạn, trò chuyện
					và tương tác giữa người dùng. Gói này phụ thuộc vào *auth* để xác
					thực danh tính người dùng và *infrastructure* để gắn kết thông
					tin vị trí địa lý.

				
-  **Gói *event**:* Quản lý lịch trình sự kiện và các
					phòng livestream. Gói này sử dụng dữ liệu không gian từ *infrastructure*,
					cơ chế xác thực từ *auth* và thông tin tương tác người dùng từ
					*social*.

				
-  **Gói *state**:* Đảm nhiệm việc đồng bộ trạng thái và
					vị trí của các thực thể theo thời gian thực. Gói này phụ thuộc vào *auth*
					để định danh thực thể và *infrastructure* để khai thác dữ liệu bản
					đồ phục vụ quá trình di chuyển.
			

		
-  **Tầng 3 -- Analytics Layer:**

			Đây là tầng xử lý các chức năng phân tích nâng cao, được xây dựng dựa trên
			dữ liệu thu thập từ các miền nghiệp vụ cốt lõi.

			
				
-  **Gói *prediction**:* Cung cấp khả năng dự báo quỹ đạo
					di chuyển bằng các mô hình trí tuệ nhân tạo. Gói này khai thác dữ liệu lịch
					sử từ *state* và thông tin về các vị trí tiềm năng từ *infrastructure*.

				
-  **Gói *heatmap**:* Thực hiện tính toán mật độ phân bố
					sinh viên theo không gian và thời gian. Gói này sử dụng dữ liệu vị trí từ
					*state* và ánh xạ kết quả lên các khu vực tương ứng thông qua
					dữ liệu được cung cấp bởi *infrastructure*.
			
	

	Nhìn chung, biểu đồ gói cho thấy kiến trúc hệ thống được tổ chức theo mô hình
	phân tầng với luồng phụ thuộc một chiều rõ ràng. Các gói ở tầng phân tích chỉ phụ
	thuộc vào các gói nghiệp vụ cốt lõi, trong khi toàn bộ các miền nghiệp vụ đều
	dựa trên các dịch vụ nền tảng ở tầng hạ tầng. Cách tổ chức này tuân thủ nguyên tắc
	phụ thuộc một chiều, theo đó các thành phần ở tầng thấp không phụ thuộc vào
	các thành phần ở tầng cao. Nhờ vậy, hệ thống đạt được mức độ kết dính cao, giảm
	sự phụ thuộc giữa các miền nghiệp vụ, đồng thời nâng cao khả năng bảo trì, mở rộng
	và phát triển độc lập của từng thành phần.

	### Thiết kế chi tiết gói

	Hệ thống được cấu thành từ nhiều dịch vụ độc lập, mỗi dịch vụ quản lý các gói nghiệp
	vụ riêng biệt. Để minh họa rõ nét kiến trúc phân lớp, các mối quan hệ thành phần
	cũng như cơ chế giao tiếp liên dịch vụ, toàn bộ các domain nghiệp vụ cốt lõi
	được phân chia thành ba nhóm gói tiêu biểu.

	**1. Nhóm thiết lập bộ khung không gian số**

	Nhóm này bao gồm các gói \texttt{building}, \texttt{event} và \texttt{campusway},
	đóng vai trò định hình bối cảnh vật lý và các hoạt động của khuôn viên trường trên
	không gian số.
	
![Biểu đồ thiết kế chi tiết nhóm bộ khung không gian số](Hinhve/pkg-spatial-backbone.png)
*Hình: Biểu đồ thiết kế chi tiết nhóm bộ khung không gian số*

	Trong biểu đồ trên, \texttt{Building} có mối quan hệ hợp thành
	với \texttt{Room}. Về mặt thiết kế hệ thống, các luồng nghiệp vụ đều tuân thủ chặt
	chẽ nguyên lý Dependency Inversion thông qua việc trừu tượng
	hóa các interface cốt lõi như \texttt{IBuildingService} và \texttt{IEventService}.
	Song song đó, \texttt{IndoorEvent} và \texttt{OutdoorEvent} là các lớp kế thừa từ
	lớp trừu tượng \texttt{Event}, thể hiện tính đa hình. Mỗi \texttt{Event} có thể
	kết tập (Aggregation) nhiều \texttt{EventAttendance} để lưu vết số lượng người
	tham gia sự kiện. Đáng chú ý, cả \texttt{EventService} và \texttt{CampusWayService}
	đều phụ thuộc vào \texttt{IBuildingService} để truy xuất tọa độ và tham chiếu không
	gian nhằm định vị sự kiện cũng như các tuyến đường đi lại trong khuôn viên.

	**2. Nhóm thu thập hành vi người dùng**

	Nhóm thứ hai bao gồm các gói \texttt{social}, \texttt{journey} và \texttt{post},
	quản lý toàn bộ các tương tác và trải nghiệm cá nhân của sinh viên trong nền
	tảng.
	
![Biểu đồ thiết kế chi tiết nhóm thu thập hành vi người dùng](Hinhve/pkg-behaviour-collection.png)
*Hình: Biểu đồ thiết kế chi tiết nhóm thu thập hành vi người dùng*

	Tương tự nhóm 1, việc tương tác giữa các tầng Controller, Service và Repository
	đều được thực hiện qua các giao diện hợp đồng (\texttt{IPostService}, \texttt{IJourneyService},
	\texttt{IFriendService}) để giảm độ liên kết chặt. Ở mức cấu trúc dữ liệu, một
	\texttt{Post} có thể hợp thành từ nhiều \texttt{Comment} và \texttt{MediaAttachment}
	(ảnh, video). Tương tự, \texttt{Journey} được cấu trúc từ danh sách các điểm lịch
	sử \texttt{JourneyItem}. Nhờ cơ chế giao tiếp linh hoạt, \texttt{IJourneyService}
	có thể sinh ra trải nghiệm hành trình mới và tạo bài đăng chia sẻ liên kết trực
	tiếp tới \texttt{Post}.

	**3. Nhóm đồng bộ không gian thời gian thực**

	Nhóm cuối cùng đặc trưng cho cơ chế đồng bộ tọa độ thời gian thực và quản lý
	vùng quan tâm không gian, nổi bật với sự giao tiếp liên hệ thống thông qua gRPC.
	
![Biểu đồ thiết kế chi tiết nhóm State và Matcher](Hinhve/pkg-state-matcher.png)
*Hình: Biểu đồ thiết kế chi tiết nhóm State và Matcher*

	Về mặt mô hình logic, \texttt{WebSocketGateway} chịu trách nhiệm tiếp nhận dữ
	liệu vị trí và phân phối xuống \texttt{IDisseminationService}. Lớp dịch vụ này
	tiếp tục ủy quyền qua \texttt{MatcherGrpcClient} để đẩy dữ liệu sang
	\texttt{MatcherGrpcController} của phân hệ tính toán không gian. Bên trong gói \texttt{interest\_matcher},
	thuật toán phân vùng được đảm nhiệm bởi \texttt{ZoneService}, dịch vụ này trực tiếp
	tính toán, quản lý không gian và chia nhỏ bản đồ vật lý thành cấu trúc dữ liệu
	lưới \texttt{SpatialGrid} cấu thành từ nhiều \texttt{GridCell}.
	## Thiết kế chi tiết

	### Thiết kế giao diện

	Thiết kế giao diện của hệ thống được chia thành hai phần chính: ứng dụng di động
	dành cho sinh viên và hệ thống quản trị. Các wireframe được xây dựng với mục
	tiêu trực quan, hiện đại và tối ưu hóa thao tác.

	#### Giao diện ứng dụng di động

	Ứng dụng di động đóng vai trò là trung tâm tương tác cốt lõi, được thiết kế để sinh viên trực tiếp sử dụng, qua đó hệ thống tiến hành thu thập dữ liệu vị trí phục vụ quá trình phân tích. Giao diện tập trung
	vào bản đồ khuôn viên giúp sinh viên dễ dàng định vị, theo dõi và tham gia sự
	kiện theo thời gian thực, cũng như ghi nhận lộ trình di chuyển của bản thân.
	Hình [fig:mobile_wireframes] minh họa wireframe của ba màn hình quan trọng nhất trên ứng
	dụng.

	
![Màn hình chính](Hinhve/Main-app.png)
*Hình: Màn hình chính*

	#### Giao diện quản trị viên

	Chức năng Simulation hỗ trợ thiết lập và mô phỏng các kịch bản khác nhau, chẳng
	hạn như ngày hội định hướng hoặc các kỳ thi lớn, qua đó giúp nhà quản lý đánh
	giá trước tác động và xây dựng phương án điều tiết lưu lượng sinh viên phù hợp.
	
![Wireframe giao diện tính năng mô phỏng](Hinhve/Simulation.png)
*Hình: Wireframe giao diện tính năng mô phỏng*

	Giao diện Journey Predict tích hợp các kết quả phân tích dữ liệu và mô hình dự
	báo nhằm mô phỏng xu hướng di chuyển của sinh viên trong tương lai. Thông tin này
	hỗ trợ nhà quản lý trong việc quy hoạch không gian và đưa ra các quyết định vận
	hành phù hợp đối với khuôn viên trường.
	
![Wireframe giao diện dự báo hành trình](Hinhve/Journey-predict.png)
*Hình: Wireframe giao diện dự báo hành trình*

	### Thiết kế lớp

	Phần này trình bày chi tiết thiết kế lớp của một số thành phần chủ đạo trong
	hệ thống, bao gồm các lớp lưu trữ thông tin và các lớp dịch vụ xử lý nghiệp vụ.

	#### Lớp User và UserService

	**Lớp User**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp User}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Thuộc tính** & **Kiểu dữ liệu** &
			**Mô tả** \\ \midrule id & Long & Mã định danh người dùng \\ phonenumber
			& String & Số điện thoại sử dụng làm tên đăng nhập \\ password & String & Mật
			khẩu tài khoản đã được mã hóa \\ username & String & Tên hiển thị của người
			dùng \\ avatar & String & Đường dẫn ảnh đại diện người dùng \\ coverImage &
			String & Đường dẫn ảnh bìa cá nhân \\ description & String & Thông tin mô tả
			ngắn về người dùng \\ status & Enum & Trạng thái hoạt động của tài khoản \\
			role & Enum & Vai trò và quyền hạn trong hệ thống \\ \bottomrule
		\end{tabularx}
	\end{table}

	**Lớp UserService**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp UserService}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Phương thức** & **Tham số** &
			**Mô tả** \\ \midrule getUsers() & - & Lấy danh sách người dùng trong hệ
			thống \\ getUserById() & userId & Tìm kiếm người dùng theo mã định danh \\ createUser()
			& User & Tạo tài khoản người dùng mới \\ updateUser() & User & Cập nhật thông
			tin cá nhân người dùng \\ deleteUser() & userId & Xóa tài khoản khỏi hệ thống
			\\ \bottomrule
		\end{tabularx}
	\end{table}

	#### Lớp Building và BuildingService

	**Lớp Building**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp Building}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Thuộc tính** & **Kiểu dữ liệu** &
			**Mô tả** \\ \midrule id & Long & Mã định danh tòa nhà \\ name & String
			& Tên tòa nhà \\ category & Enum & Loại tòa nhà trong hệ thống \\ mapId & Long
			& Bản đồ mà tòa nhà thuộc về \\ location & Object & Thông tin vị trí của tòa
			nhà \\ roomCount & Integer & Số lượng phòng trong tòa nhà \\ floorCount & Integer
			& Số tầng của tòa nhà \\ isActive & Boolean & Trạng thái hoạt động của tòa nhà
			\\ \bottomrule
		\end{tabularx}
	\end{table}

	**Lớp BuildingService**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp BuildingService}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Phương thức** & **Tham số** &
			**Mô tả** \\ \midrule getBuildings() & - & Lấy danh sách các tòa nhà \\
			getBuildingById() & buildingId & Lấy thông tin chi tiết tòa nhà \\ createBuilding()
			& Building & Thêm mới tòa nhà \\ updateBuilding() & Building & Cập nhật thông
			tin tòa nhà \\ deleteBuilding() & buildingId & Xóa tòa nhà khỏi hệ thống \\
			\bottomrule
		\end{tabularx}
	\end{table}

	#### Lớp Event và EventService

	**Lớp Event**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp Event}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Thuộc tính** & **Kiểu dữ liệu** &
			**Mô tả** \\ \midrule id & Long & Mã định danh sự kiện \\ name & String
			& Tên sự kiện \\ description & String & Nội dung mô tả sự kiện \\ mapId & Long
			& Khu vực bản đồ tổ chức sự kiện \\ startTime & DateTime & Thời gian bắt đầu
			sự kiện \\ endTime & DateTime & Thời gian kết thúc sự kiện \\ eventType & Enum
			& Loại sự kiện \\ status & Enum & Trạng thái của sự kiện \\ \bottomrule
		\end{tabularx}
	\end{table}

	**Lớp EventService**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp EventService}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Phương thức** & **Tham số** &
			**Mô tả** \\ \midrule getEvents() & - & Lấy danh sách sự kiện \\ getEventById()
			& eventId & Xem thông tin chi tiết sự kiện \\ createEvent() & Event & Tạo sự
			kiện mới \\ updateEvent() & Event & Cập nhật thông tin sự kiện \\ deleteEvent()
			& eventId & Xóa sự kiện \\ \bottomrule
		\end{tabularx}
	\end{table}

	#### Lớp RecurringEvent và RecurringEventService

	**Lớp RecurringEvent**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp RecurringEvent}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Thuộc tính** & **Kiểu dữ liệu** &
			**Mô tả** \\ \midrule id & Long & Mã định danh sự kiện định kỳ \\ name
			& String & Tên sự kiện định kỳ \\ description & String & Nội dung mô tả sự kiện
			định kỳ \\ cronExpression & String & Biểu thức cron lặp lại lịch \\ durationMinutes
			& Integer & Thời lượng sự kiện tính bằng phút \\ mapId & Long & Mã bản đồ nơi
			diễn ra sự kiện \\ roomId & Long & Mã phòng diễn ra sự kiện \\ status & Enum
			& Trạng thái hoạt động \\ \bottomrule
		\end{tabularx}
	\end{table}

	**Lớp RecurringEventService**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp RecurringEventService}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Phương thức** & **Tham số** &
			**Mô tả** \\ \midrule getRecurringEvents() & - & Lấy danh sách sự kiện
			định kỳ \\ getRecurringEventById() & eventId & Xem chi tiết sự kiện định kỳ
			\\ createRecurringEvent() & RecurringEvent & Cấu hình lịch lặp mới \\ updateRecurringEvent()
			& RecurringEvent & Cập nhật cấu hình sự kiện lặp \\ deleteRecurringEvent() &
			eventId & Hủy sự kiện định kỳ \\ generateOccurrences() & eventId & Sinh danh
			sách sự kiện chi tiết \\ \bottomrule
		\end{tabularx}
	\end{table}

	#### Lớp Journey và JourneyService

	**Lớp Journey**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp Journey}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Thuộc tính** & **Kiểu dữ liệu** &
			**Mô tả** \\ \midrule id & Long & Mã định danh hành trình \\ userId & Long
			& Mã người dùng sở hữu hành trình \\ title & String & Tiêu đề hành trình \\
			description & String & Nội dung tóm tắt hành trình \\ journeyDate & Date & Ngày
			thực hiện hành trình \\ status & Enum & Trạng thái lưu trữ (Bản nháp, Công khai)
			\\ items & List & Danh sách các địa điểm dừng chân \\ pathCoordinates &
			Json & Dữ liệu tọa độ di chuyển \\ \bottomrule
		\end{tabularx}
	\end{table}

	**Lớp JourneyService**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp JourneyService}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Phương thức** & **Tham số** &
			**Mô tả** \\ \midrule generateJourneyDraft() & userId, ... & Nội suy
			dữ liệu GPS thành hành trình \\ getUserJourneys() & userId & Lấy lịch sử
			hành trình của một người dùng \\ getJourneyById() & journeyId & Lấy thông
			tin chi tiết hành trình \\ createJourney() & Journey & Tạo mới một hành
			trình \\ updateJourney() & Journey & Chỉnh sửa thông tin hành trình \\
			deleteJourney() & journeyId & Xóa hành trình khỏi hệ thống \\ \bottomrule
		\end{tabularx}
	\end{table}

	#### Lớp Post và PostService

	**Lớp Post**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp Post}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Thuộc tính** & **Kiểu dữ liệu** &
			**Mô tả** \\ \midrule id & Long & Mã định danh bài viết \\ userId & Long
			& Người tạo bài viết \\ content & String & Nội dung văn bản của bài viết \\
			status & Enum & Trạng thái hiển thị \\ location & Point & Tọa độ nơi người dùng
			đăng bài \\ eventId & Long & Khóa ngoại liên kết sự kiện \\ buildingId & Long
			& Khóa ngoại liên kết tòa nhà \\ canComment & Boolean & Cho phép bình luận bài
			viết \\ \bottomrule
		\end{tabularx}
	\end{table}

	**Lớp PostService**
	\begin{table}[H]
		\centering
		\caption{Thiết kế lớp PostService}
		
		\begin{tabularx}
			{\textwidth}{llX} \toprule **Phương thức** & **Tham số** &
			**Mô tả** \\ \midrule getPosts() & pageable & Lấy danh sách bài viết chung
			\\ getPostsByUser() & userId & Lấy bài đăng của một người dùng cụ thể \\ getPostsByEvent()
			& eventId & Lấy các bài đăng liên quan đến sự kiện \\ createPost() & Post &
			Thêm bài đăng mới \\ updatePost() & Post & Sửa nội dung bài đăng \\ deletePost()
			& postId & Xóa bài đăng \\ \bottomrule
		\end{tabularx}
	\end{table}

	### Thiết kế cơ sở dữ liệu

	#### Sơ đồ thực thể liên kết ERD

	Mô hình thực thể liên kết (Entity-Relationship Diagram --- ERD) thể hiện cấu trúc
	tổng quát của cơ sở dữ liệu, phân định rõ ràng các thực thể tĩnh, các thực thể động
	và mạng lưới tương tác xã hội của sinh viên. Hình [fig:erd_diagram] trình
	bày trực quan các mối quan hệ giữa các bảng quan trọng trong hệ thống.

	
![Sơ đồ thực thể liên kết (ERD) của Hust Simulator](Hinhve/ERD.png)
*Hình: Sơ đồ thực thể liên kết (ERD) của Hust Simulator*

	Hệ thống Hust Simulator được cấu trúc thành 6 phân lớp chính nhằm quản lý toàn vẹn
	dữ liệu từ không gian vật lý, sự kiện động cho đến các hoạt động xã hội của
	người dùng:

	
		
-  **Identity Layer**: Là trung tâm của hệ thống định danh,
			đóng vai trò chủ thể trong hầu hết các hoạt động. Một User có thể:
			
				
-  Đóng góp nội dung thông qua bài đăng (Post) và bình luận (Comment).

				
-  Tương tác nội bộ bằng cách tham gia các cuộc hội thoại (Conversation)
					và gửi tin nhắn (Message).

				
-  Tiếp nhận các thông báo (Notification) từ hệ thống.

				
-  Gắn với một trạng thái hoạt động tức thời (UserState) và sở hữu lịch
					sử di chuyển (Journey).
			

		
-  **Static Twin Layer**:
			Các thực thể tĩnh trong không gian số.
			
				
-  Bản đồ (VirtualMap) chứa nhiều tòa nhà (Building), nút giao thông
					(CampusNode) và các tuyến đường (CampusWay).

				
-  Mỗi tòa nhà (Building) được chia nhỏ thành nhiều phòng
					(Room).

				
-  Nút giao thông (CampusNode) và các tuyến đường (CampusWay).
			

		
-  **Dynamic Twin Layer**: Quản lý
			các hoạt động và trạng thái có tính thời vụ.
			
				
-  Sự kiện (Event) được tổ chức tại phạm vi bản đồ, trong
					một tòa nhà, hoặc một phòng cụ thể. Sự kiện lặp lại (RecurringEvent)
					kế thừa trực tiếp từ Event.

				
-  Trạng thái người dùng (UserState) thể hiện việc sinh viên đang ở không
					gian nào hoặc đang tham gia (attends) sự kiện gì ngay tại thời điểm hiện
					tại.
			

		
-  **Location Layer**: Vị trí
			của sinh viên.
			
				
-  Điểm dừng chân (CheckinSequence) ghi nhận lại việc người dùng đã ghé
					thăm một địa điểm (CampusNode, Room) hay tham gia sự kiện (Event).

				
-  Hành trình (Journey) là một tập hợp được cấu thành từ chuỗi các điểm
					dừng chân (CheckinSequence) này.
			

		
-  **Social \& Communication Layer**: Quản lý nội dung chia sẻ và hội thoại.
			
				
-  Một bài viết (Post) có thể dùng để chia sẻ một sự kiện (Event) hoặc
					một chuyến hành trình (Journey).

				
-  Bài viết sẽ nhận các bình luận (Comment). Hệ thống bình luận cho
					phép trả lời lẫn nhau tạo thành luồng thảo luận.

				
-  Cuộc trò chuyện (Conversation) đóng vai trò nhóm các tin nhắn (Message)
					lại với nhau để người dùng trao đổi trực tiếp.
			
	

	#### Mô tả các bảng dữ liệu

	Dựa trên sơ đồ thực thể E-R đã trình bày ở mục (a), dưới đây là các bảng tiêu biểu
	mô tả các trường dữ liệu chính:

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{users}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh của người dùng trong hệ
			thống. \\ \hline username & VARCHAR & Tên hiển thị của người dùng trên
			nền tảng. \\ \hline phonenumber & VARCHAR & Số điện thoại dùng để đăng nhập
			và định danh tài khoản. \\ \hline password & VARCHAR & Mật khẩu của người
			dùng sau khi được băm và mã hóa. \\ \hline avatar & TEXT & Đường dẫn tới ảnh
			đại diện của người dùng. \\ \hline cover\_image & TEXT & Đường dẫn tới ảnh
			bìa trang cá nhân. \\ \hline description & TEXT & Thông tin giới thiệu hoặc
			mô tả ngắn về người dùng. \\ \hline role & VARCHAR & Vai trò của người
			dùng trong hệ thống\\ \hline status & VARCHAR & Trạng thái tài khoản.
			\\ \hline last\_login & TIMESTAMP & Thời diểm đăng nhập gần nhất của người
			dùng. \\ \hline email & VARCHAR & Địa chỉ thư điện tử của người dùng, phục
			vụ khôi phục tài khoản và gửi thông báo. \\ \hline gender & VARCHAR & Giới
			tính của người dùng. \\ \hline created\_at & TIMESTAMP & Thời điểm tài
			khoản được tạo. \\ \hline updated\_at & TIMESTAMP & Thời điểm thông tin được cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{virtual\_maps}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của bản đồ số. \\
			\hline name & VARCHAR & Tên của bản đồ. \\ \hline description & TEXT & Thông
			tin mô tả về khu vực được biểu diễn. \\ \hline coordinates & GEOMETRY & Đa
			giác biểu diễn phạm vi của bản đồ trong không gian thực. \\ \hline center\_latitude
			& DOUBLE & Vĩ độ của tâm bản đồ. \\ \hline center\_longitude & DOUBLE &
			Kinh độ của tâm bản đồ. \\ \hline zoom\_level & INTEGER & Mức phóng đại mặc
			định khi hiển thị. \\ \hline version & INTEGER & Phiên bản của dữ liệu bản
			đồ. \\ \hline is\_active & BOOLEAN & Cho biết bản đồ hiện có được sử dụng hay
			không. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản ghi. \\ \hline
			updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{buildings}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của tòa nhà. \\
			\hline name & VARCHAR & Tên đầy đủ của tòa nhà. \\ \hline code & VARCHAR & Mã
			viết tắt của tòa nhà \\ \hline description & TEXT & Thông tin mô tả của tòa
			nhà. \\ \hline building\_type & VARCHAR & Loại công trình, \\ \hline number\_of\_floors
			& INTEGER & Tổng số tầng của tòa nhà. \\ \hline map\_id & UUID & Bản đồ mà
			tòa nhà thuộc về. \\ \hline coordinates & GEOMETRY & phạm vi của tòa nhà.
			\\ \hline latitude & DOUBLE & Vĩ độ của tâm tòa nhà. \\ \hline longitude & DOUBLE
			& Kinh độ của tâm tòa nhà. \\ \hline is\_active & BOOLEAN & Trạng thái
			hoạt động của tòa nhà. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản
			ghi. \\ \hline updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{rooms}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của phòng. \\
			\hline name & VARCHAR & Tên hiển thị của phòng. \\ \hline room\_code & VARCHAR
			& Mã phòng \\ \hline building\_id & UUID & Tòa nhà chứa phòng. \\ \hline
			floor & INTEGER & Tầng mà phòng nằm trên đó. \\ \hline room\_type & VARCHAR
			& Loại phòng \\ \hline capacity & INTEGER & Sức chứa tối đa của phòng. \\
			\hline description & TEXT & Thông tin mô tả của phòng. \\ \hline coordinates
			& GEOMETRY & phạm vi của phòng trong không gian. \\ \hline status &
			VARCHAR & Trạng thái sử dụng của phòng. \\ \hline is\_active & BOOLEAN & Cho
			biết phòng còn được sử dụng hay không. \\ \hline created\_at & TIMESTAMP &
			Thời điểm tạo bản ghi. \\ \hline updated\_at & TIMESTAMP & Thời điểm cập nhật
			gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{campus\_nodes}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của nút trong đồ thị
			không gian. \\ \hline name & VARCHAR & Tên của điểm mốc. \\ \hline node\_type
			& VARCHAR & Loại nút như cửa ra vào, cầu thang, giao lộ hoặc thang máy. \\
			\hline latitude & DOUBLE & Vĩ độ của nút. \\ \hline longitude & DOUBLE & Kinh
			độ của nút. \\ \hline z\_coordinate & DOUBLE & Độ cao của nút trong không
			gian ba chiều. \\ \hline floor & INTEGER & Tầng chứa nút. \\ \hline
			building\_id & UUID & Tòa nhà chứa nút. \\ \hline accessible & BOOLEAN & Cho
			biết người khuyết tật có thể tiếp cận nút này hay không. \\ \hline
			description & TEXT & Thông tin mô tả của nút. \\ \hline is\_active & BOOLEAN
			& Trạng thái hoạt động của nút. \\ \hline created\_at & TIMESTAMP & Thời
			điểm tạo bản ghi. \\ \hline updated\_at & TIMESTAMP & Thời điểm cập nhật gần
			nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{campus\_ways}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của cạnh trong đồ thị.
			\\ \hline name & VARCHAR & Tên của đoạn đường. \\ \hline way\_type & VARCHAR
			& Loại đường như walkway, hallway, stair hoặc elevator. \\ \hline
			coordinates & LINESTRING & Chuỗi tọa độ biểu diễn hình học của đoạn đường. \\
			\hline distance\_meters & DOUBLE & Chiều dài thực tế của đoạn đường (mét).
			\\ \hline weight & DOUBLE & Trọng số phục vụ thuật toán tìm đường. \\ \hline
			floor & INTEGER & Tầng mà đoạn đường thuộc về. \\ \hline accessible &
			BOOLEAN & Cho biết đoạn đường có phù hợp với người khuyết tật hay không. \\
			\hline is\_oneway & BOOLEAN & Xác định cạnh một chiều hay hai chiều. \\
			\hline is\_active & BOOLEAN & Trạng thái hoạt động của đoạn đường. \\ \hline
			created\_at & TIMESTAMP & Thời điểm tạo bản ghi. \\ \hline updated\_at &
			TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{events}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của sự kiện. \\
			\hline name & VARCHAR & Tên của sự kiện. \\ \hline description & TEXT & Thông
			tin mô tả chi tiết về sự kiện. \\ \hline thumbnail\_url & TEXT & Đường dẫn
			tới ảnh đại diện của sự kiện. \\ \hline organizer & VARCHAR & Tên đơn vị tổ
			chức sự kiện. \\ \hline created\_by & UUID & Người tạo sự kiện. \\ \hline type
			& VARCHAR & Loại sự kiện \\ \hline tags & JSONB & Danh sách nhãn phân loại của
			sự kiện. \\ \hline map\_id & UUID & Bản đồ chứa sự kiện. \\ \hline building\_id
			& UUID & Tòa nhà diễn ra sự kiện. \\ \hline room\_id & UUID & Phòng tổ
			chức sự kiện. \\ \hline start\_time & TIMESTAMP & Thời điểm bắt đầu sự kiện.
			\\ \hline end\_time & TIMESTAMP & Thời điểm kết thúc sự kiện. \\ \hline max\_participants
			& INTEGER & Số lượng người tham gia tối đa. \\ \hline status & VARCHAR &
			Trạng thái của sự kiện: scheduled, ongoing, completed hoặc cancelled. \\
			\hline min\_x & DOUBLE & Biên trái của vùng ảnh hưởng. \\ \hline min\_y & DOUBLE
			& Biên dưới của vùng ảnh hưởng. \\ \hline max\_x & DOUBLE & Biên phải của
			vùng ảnh hưởng. \\ \hline max\_y & DOUBLE & Biên trên của vùng ảnh hưởng.
			\\ \hline is\_active & BOOLEAN & Cho biết sự kiện còn hiệu lực hay không. \\
			\hline created\_at & TIMESTAMP & Thời điểm tạo bản ghi. \\ \hline updated\_at
			& TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{recurring\_events}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của sự kiện định kỳ.
			\\ \hline name & VARCHAR & Tên sự kiện định kỳ. \\ \hline description & TEXT
			& Thông tin mô tả của sự kiện. \\ \hline cron\_expression & VARCHAR & Biểu
			thức cron xác định chu kỳ lặp lại của sự kiện. \\ \hline timezone & VARCHAR
			& Múi giờ áp dụng cho biểu thức cron. \\ \hline duration\_minutes &
			INTEGER & Thời lượng diễn ra của mỗi lần lặp. \\ \hline next\_run\_time & TIMESTAMP
			& Thời điểm dự kiến của lần thực thi tiếp theo. \\ \hline created\_by &
			UUID & Người tạo sự kiện định kỳ. \\ \hline map\_id & UUID & Bản đồ chứa sự
			kiện. \\ \hline building\_id & UUID & Tòa nhà diễn ra sự kiện. \\ \hline room\_id
			& UUID & Phòng tổ chức sự kiện. \\ \hline max\_participants & INTEGER & Số
			lượng người tham gia tối đa. \\ \hline status & VARCHAR & Trạng thái của sự
			kiện định kỳ. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản ghi. \\
			\hline updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{user\_states}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của trạng thái người
			dùng. \\ \hline user\_id & UUID & Người dùng mà trạng thái này thuộc về. \\
			\hline activity\_state & VARCHAR & Trạng thái hoạt động hiện tại của người dùng.
			\\ \hline map\_id & UUID & Bản đồ mà người dùng đang hiện diện. \\ \hline building\_id
			& UUID & Tòa nhà hiện tại của người dùng. \\ \hline room\_id & UUID &
			Phòng mà người dùng đang ở. \\ \hline node\_id & UUID & Nút gần nhất của người
			dùng trên đồ thị không gian. \\ \hline event\_id & UUID & Sự kiện mà người
			dùng đang tham gia. \\ \hline latitude & DOUBLE & Vĩ độ hiện tại của người dùng.
			\\ \hline longitude & DOUBLE & Kinh độ hiện tại của người dùng. \\ \hline heading
			& DOUBLE & Hướng di chuyển hiện tại của người dùng (độ). \\ \hline speed &
			DOUBLE & Tốc độ di chuyển hiện tại của người dùng (m/s). \\ \hline state\_version
			& BIGINT & Phiên bản của trạng thái phục vụ đồng bộ thời gian thực. \\
			\hline session\_data & JSONB & Thông tin ngữ cảnh và dữ liệu phiên làm
			việc của người dùng. \\ \hline entered\_at & TIMESTAMP & Thời điểm bắt đầu trạng
			thái hiện tại. \\ \hline last\_updated & TIMESTAMP & Thời điểm nhận dữ
			liệu cập nhật gần nhất. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản
			ghi. \\ \hline updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{user\_locations}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của bản ghi vị trí.
			\\ \hline user\_id & UUID & Người dùng sở hữu dữ liệu vị trí. \\ \hline latitude
			& DOUBLE & Vĩ độ của vị trí được ghi nhận. \\ \hline longitude & DOUBLE &
			Kinh độ của vị trí được ghi nhận. \\ \hline accuracy & DOUBLE & Sai số ước lượng
			của vị trí (mét). \\ \hline speed & DOUBLE & Tốc độ di chuyển tại thời
			điểm ghi nhận (m/s). \\ \hline heading & DOUBLE & Hướng di chuyển của người
			dùng (độ). \\ \hline source & VARCHAR & Nguồn dữ liệu vị trí như GPS, WiFi hoặc
			BLE. \\ \hline timestamp & TIMESTAMP & Thời điểm dữ liệu vị trí được ghi
			nhận. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản ghi. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{checkin\_sequences}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của lần check-in.
			\\ \hline user\_id & UUID & Người dùng thực hiện check-in. \\ \hline node\_id
			& UUID & Điểm mốc gần nhất trên đồ thị không gian. \\ \hline building\_id
			& UUID & Tòa nhà mà người dùng check-in. \\ \hline room\_id & UUID & Phòng mà
			người dùng check-in. \\ \hline event\_id & UUID & Sự kiện liên quan tới
			lần check-in. \\ \hline sequence\_order & INTEGER & Thứ tự của điểm check-in
			trong chuỗi hành trình. \\ \hline duration\_minutes & INTEGER & Thời gian
			lưu lại tại điểm check-in (phút). \\ \hline timestamp & TIMESTAMP & Thời điểm
			thực hiện check-in. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản
			ghi. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{journeys}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline id & UUID & Định danh duy nhất của hành trình. \\
			\hline user\_id & UUID & Người sở hữu hành trình. \\ \hline title & VARCHAR
			& Tiêu đề của hành trình. \\ \hline description & TEXT & Thông tin mô tả
			về hành trình. \\ \hline journey\_date & DATE & Ngày diễn ra hành trình.
			\\ \hline start\_time & TIMESTAMP & Thời điểm bắt đầu hành trình. \\ \hline
			end\_time & TIMESTAMP & Thời điểm kết thúc hành trình. \\ \hline total\_distance
			& DOUBLE & Tổng quãng đường di chuyển (mét). \\ \hline total\_checkins & INTEGER
			& Tổng số điểm check-in trong hành trình. \\ \hline video\_url & TEXT &
			Đường dẫn tới video recap được tạo tự động. \\ \hline music\_url & TEXT & Đường
			dẫn tới nhạc nền của video. \\ \hline template\_id & VARCHAR & Mẫu dựng
			video được sử dụng. \\ \hline visibility & VARCHAR & Phạm vi hiển thị của hành
			trình (private, friends, public). \\ \hline status & VARCHAR & Trạng thái
			xử lý của hành trình. \\ \hline created\_at & TIMESTAMP & Thời điểm tạo bản
			ghi. \\ \hline updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{posts}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline

			id & UUID & Định danh duy nhất của bài đăng. \\ \hline

			user\_id & UUID & Người tạo bài đăng. \\ \hline

			content & TEXT & Nội dung văn bản của bài đăng. \\ \hline

			image\_urls & JSONB & Danh sách ảnh đính kèm. \\ \hline

			video\_url & TEXT & Đường dẫn video đính kèm. \\ \hline

			journey\_id & UUID & Hành trình được chia sẻ (nếu có). \\ \hline

			location & GEOMETRY & Vị trí liên quan tới bài đăng. \\ \hline

			visibility & VARCHAR & Phạm vi hiển thị: private, friends hoặc public. \\ \hline

			allow\_comment & BOOLEAN & Cho phép bình luận hay không. \\ \hline

			allow\_edit & BOOLEAN & Cho phép chỉnh sửa bài đăng. \\ \hline

			status & VARCHAR & Trạng thái bài đăng: active, hidden hoặc deleted. \\ \hline

			created\_at & TIMESTAMP & Thời điểm tạo bài đăng. \\ \hline

			updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{comments}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline

			id & UUID & Định danh duy nhất của bình luận. \\ \hline

			post\_id & UUID & Bài đăng chứa bình luận. \\ \hline

			user\_id & UUID & Người tạo bình luận. \\ \hline

			parent\_comment\_id & UUID & Bình luận cha, hỗ trợ trả lời bình luận. \\
			\hline

			content & TEXT & Nội dung bình luận. \\ \hline

			status & VARCHAR & Trạng thái của bình luận. \\ \hline

			created\_at & TIMESTAMP & Thời điểm tạo bình luận. \\ \hline

			updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{conversations}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline

			id & UUID & Định danh duy nhất của cuộc trò chuyện. \\ \hline

			conversation\_type & VARCHAR & Loại cuộc trò chuyện: private hoặc group.
			\\ \hline

			title & VARCHAR & Tên nhóm trò chuyện. \\ \hline

			avatar & TEXT & Ảnh đại diện của cuộc trò chuyện. \\ \hline

			created\_by & UUID & Người tạo cuộc trò chuyện. \\ \hline

			last\_message\_id & UUID & Tin nhắn mới nhất. \\ \hline

			last\_message\_at & TIMESTAMP & Thời điểm gửi tin nhắn gần nhất. \\ \hline

			is\_deleted & BOOLEAN & Trạng thái xóa mềm. \\ \hline

			created\_at & TIMESTAMP & Thời điểm tạo cuộc trò chuyện. \\ \hline

			updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}

	\begin{table}[H]
		\centering
		\caption{Bảng \texttt{messages}}
		
		\begin{tabularx}
			{\textwidth}{|l|l|X|} \hline **Trường** & **Kiểu dữ liệu** &
			**Mô tả** \\ \hline

			id & UUID & Định danh duy nhất của tin nhắn. \\ \hline

			conversation\_id & UUID & Cuộc trò chuyện chứa tin nhắn. \\ \hline

			sender\_id & UUID & Người gửi tin nhắn. \\ \hline

			message\_type & VARCHAR & Loại tin nhắn: text, image, video hoặc file. \\
			\hline

			content & TEXT & Nội dung tin nhắn. \\ \hline

			attachment\_url & TEXT & Tệp đính kèm của tin nhắn. \\ \hline

			reply\_to\_message\_id & UUID & Tin nhắn được trả lời. \\ \hline

			is\_read & BOOLEAN & Đánh dấu đã đọc. \\ \hline

			is\_deleted & BOOLEAN & Đánh dấu xóa mềm. \\ \hline

			sent\_at & TIMESTAMP & Thời điểm gửi tin nhắn. \\ \hline

			updated\_at & TIMESTAMP & Thời điểm cập nhật gần nhất. \\ \hline
		\end{tabularx}
	\end{table}



<!-- END OF 4_Phan_tich_thiet_ke.tex -->
---


<!-- START OF 5_Trien_khai_danh_gia.tex (Chương 5: Triển khai và đánh giá) -->



	## Xây dựng ứng dụng

	### Kiến trúc Microservice với bảy service xử lý các nghiệp vụ khác nhau

	Hệ thống đã được triển khai thành công dựa trên kiến trúc Microservices. Để giải quyết các bài toán phức tạp của môi trường Digital Twin, toàn bộ chức năng được phân tách thành bảy service độc lập, mỗi service chịu trách nhiệm cho một nhóm nghiệp vụ chuyên biệt. Cấu trúc tổng thể của hệ thống được minh họa như trong Hình [fig:kientruc_microservices].

	
![Sơ đồ kiến trúc Microservices](Hinhve/KienTruc.png)
*Hình: Sơ đồ kiến trúc Microservices*

	Chi tiết về bảy service chính của hệ thống như sau:

-  **Auth Service:** Quản lý danh tính người dùng, bao gồm đăng ký tài khoản, đăng nhập, phân quyền truy cập và xác thực các yêu cầu từ hệ thống.

-  **Social Service:** Cung cấp các chức năng tương tác xã hội như quản lý bạn bè, bài viết, bình luận, thông báo và các hoạt động cộng đồng trong nền tảng.

-  **Context Service:** Quản lý dữ liệu ngữ cảnh của môi trường Digital Twin, bao gồm thông tin các tòa nhà, phòng học, điểm quan tâm (POI), sự kiện và các thực thể không gian tĩnh.

-  **Streaming Service:** Quản lý chức năng truyền phát đa phương tiện theo thời gian thực dựa trên giao thức WebRTC. Dịch vụ này hỗ trợ hệ thống khởi tạo các phòng phát sóng trực tiếp cho sự kiện, lớp học ảo và điều phối luồng âm thanh/video tới hàng ngàn người tham gia.

-  **State Computation \& Dissemination Service:** Đảm nhiệm vai trò duy trì trạng thái liên tục của Digital Twin. Trong khi *Computation Service* chịu trách nhiệm xử lý logic vòng lặp mô phỏng và tính toán tọa độ, *Dissemination Service* chuyên quản lý hàng ngàn kết nối WebSocket để cập nhật trạng thái liên tục về các thiết bị đang trực tuyến.

-  **Interest Matcher Service:** Đóng vai trò là các Broker không gian phục vụ kiến trúc phân phối dữ liệu dựa trên vùng quan tâm (Area of Interest --- AOI). Dịch vụ này liên tục đối khớp (match) tọa độ của người dùng với các khu vực không gian nhằm định tuyến dữ liệu, đảm bảo client chỉ nhận được các bản cập nhật thực thể nằm xung quanh mình.

-  **Prediction Service:** Thu thập dữ liệu lịch sử di chuyển, xây dựng hồ sơ hành vi và thực hiện các tác vụ dự báo xu hướng phân bố người dùng trong tương lai.

Bên cạnh việc phân tách logic nghiệp vụ, hệ thống cũng áp dụng triệt để kiến trúc **Database-per-Service** ở tầng lưu trữ. Cụ thể, mỗi service sở hữu và quản lý một cơ sở dữ liệu vật lý hoàn toàn độc lập. Việc phân tán dữ liệu ở mức vật lý này đảm bảo nguyên tắc cô lập dữ liệu tuyệt đối giữa các miền nghiệp vụ, tránh tình trạng chia sẻ cơ sở dữ liệu chung gây thắt nút cổ chai. Nhờ đó, hệ thống dễ dàng mở rộng và tối ưu hóa tài nguyên phần cứng riêng biệt cho từng service khi tải lưu lượng tăng cao.

Mặc dù hệ thống được phân chia thành nhiều service ở phía backend, các thiết bị client chỉ giao tiếp thông qua một lối vào duy nhất là **API Gateway**. Cổng giao tiếp trung tâm này chịu trách nhiệm phân tích và định tuyến từng yêu cầu đến đúng service đích, xử lý Load Balancing, mã hóa bảo mật SSL/TLS và kiểm soát lưu lượng truy cập. Thiết kế này vừa giúp đơn giản hóa việc tích hợp ở phía client, vừa đảm bảo khả năng mở rộng một cách linh hoạt cho từng service thành phần ở phía sau.

	### Thư viện và công cụ sử dụng

	\begin{table}[H]
		\centering
		\caption{Các công nghệ và thư viện được sử dụng trong từng thành phần của hệ
		thống}
		 \small
		\renewcommand{\arraystretch}{1.3}
		\begin{tabular}{p{3.5cm}p{4cm}p{6.5cm}}
			\toprule **Service**                             & **Công nghệ / Thư viện** & **Mục đích sử dụng**                                    \\
			\midrule

**Core Java Services**                 & Spring JPA, Hibernate    & Ánh xạ đối tượng -- quan hệ và giao tiếp với cơ sở dữ liệu. \\
			\midrule

**Auth Service**                       & Spring Security, JWT          & Xác thực người dùng, phân quyền và cấp phát token định danh. \\
			\midrule

**Context Service**                    & gRPC (Java)                   & Giao tiếp hiệu năng cao giữa các dịch vụ.                    \\
			                                                      & JTS Topology Suite            & Hỗ trợ phép toán hình học không gian.                 \\
			\midrule

**Social Service**                     & Netty Socket.IO               & Quản lý kết nối chat thời gian thực.                         \\
			                                                      & Firebase Admin SDK            & Tích hợp Push Notification.                                  \\
																  & Firebase Storage              & Hỗ trợ upload file.                               \\
			                                                      & Hibernate Spatial             & Truy vấn và xử lý dữ liệu GIS.                               \\
			\midrule

**Streaming Service**                  & LiveKit Server SDK            & Quản lý livestream và điều phối WebRTC.                \\
			                                                      & Redis Pub/Sub                 & Đồng bộ trạng thái giữa các thành phần.                      \\
			\midrule

**State Computation \& Dissemination** & NestJS, WebSocket             & Tiếp nhận và phát tán cập nhật vị trí theo thời gian thực.   \\
			\midrule

**Interest Matcher**           & NestJS, gRPC                  & Xây dựng broker giao tiếp hiệu năng cao.                     \\
			                                                      & KD-Tree                       & Đối khớp không gian và định tuyến theo AOI.                  \\
			\midrule

**Prediction Service**                 & Python, Scikit-Learn          & Huấn luyện mô hình dự báo ngữ cảnh.    \\
			                                                      & Pandas, NumPy                 & Xử lý dữ liệu không gian -- thời gian.                        \\
			                                                      & FastAPI, Uvicorn              & Cung cấp API cho mô hình dự đoán.                            \\
			\midrule

**Admin Dashboard**                    & React, Tailwind CSS           & Xây dựng giao diện quản trị.                                 \\
			                                                      & Deck.gl, MapLibre GL          & Hiển thị bản đồ và dữ liệu không gian 3D.                    \\
			                                                      & Recharts                      & Trực quan hóa số liệu thống kê.                              \\
			\midrule

**Shared Infra**              & RabbitMQ                      & Truyền thông điệp bất đồng bộ.              \\
			                                                      & PostgreSQL, PostGIS           & Lưu trữ dữ liệu và hỗ trợ spatial index.                \\
			                                                      & Redis                         & Bộ nhớ đệm và lưu trữ trạng thái tạm thời.                   \\
			                                                      & Flyway, Springdoc             & Quản lý phiên bản database và tài liệu hóa API.         \\
			\bottomrule
		\end{tabular}
	\end{table}

	### Kết quả đạt được

	Sau quá trình nghiên cứu, thiết kế và triển khai, hệ thống Hust Simulator đã được
	xây dựng thành công và đạt được các mục tiêu kỹ thuật đặt ra ban đầu. Các kết
	quả cụ thể bao gồm:
	
		
-  **Hust Simulator Mobile App:** Hệ thống backend đã được tích hợp
			thành công với ứng dụng di động. Quá trình tích hợp phía client được thực hiện
			độc lập bởi thành viên khác trong nhóm, do đó phạm vi của đồ án này không
			đi sâu vào chi tiết kỹ thuật của phân hệ ứng dụng di động.

		
-  **Hust Simulator Server:** Hệ thống máy chủ đã được xây dựng hoàn
			chỉnh theo kiến trúc Microservices, cho phép các dịch vụ được phát triển,
			triển khai và mở rộng một cách độc lập. Các service được container hóa bằng
			Docker và được điều phối bởi Kubernetes nhằm đảm bảo khả năng tự động triển
			khai, cân bằng tải, khôi phục khi xảy ra sự cố và mở rộng linh hoạt theo nhu
			cầu sử dụng. Toàn bộ hệ thống đã được triển khai thành công trên nền tảng
			Google Cloud Platform, đáp ứng các yêu cầu về tính sẵn sàng, khả năng mở
			rộng và thuận tiện trong quá trình vận hành, bảo trì.

		
-  **Hust Simulator Admin:** Hệ thống quản trị được phát triển bằng
			React, cung cấp các chức năng phục vụ công tác quản lý, giám sát và phân
			tích dữ liệu của hệ thống. Giao diện của phân hệ này được thiết kế theo hướng
			tối giản, ưu tiên tính chính xác, hiệu quả khai thác dữ liệu và khả năng hỗ
			trợ quản trị hơn là yếu tố thẩm mỹ dành cho người dùng cuối.
	

	Bên cạnh đó, các số liệu tổng quan đánh giá quy mô dự án và hiệu năng hệ thống được
	đo lường thực tế và tổng hợp chi tiết trong Bảng [table:system_metrics].

	\begin{table}[H]
		\centering
		\caption{Tổng hợp quy mô và hiệu năng hệ thống}
		
		\begin{tabular}{>{\raggedright\arraybackslash}p{7cm} >{\raggedright\arraybackslash}p{4cm}}
			\toprule **Chỉ số đo lường**        & **Giá trị** \\
			\midrule Tổng số file mã nguồn (Backend) & $\sim 450$          \\
			Tổng số dòng mã lệnh (Backend)           & $\sim 35.000$       \\
			Tổng số file mã nguồn (Admin)            & $\sim 60$        \\
			Tổng số dòng mã lệnh (Admin)             & $\sim 6.500$        \\
			Tổng số API Endpoints                    & $\sim 160$          \\
			Tổng số bảng (Database)            & $\sim 40$           \\
			\bottomrule
		\end{tabular}
	\end{table}
	### Minh họa các chức năng chính

	#### Giao diện ứng dụng di động

	Ứng dụng di động dành cho người dùng cuối cung cấp các tính năng tương tác với
	bản đồ số, xem thông tin tòa nhà và tham gia các sự kiện. Do phân hệ ứng dụng
	client được thực hiện độc lập bởi thành viên khác trong nhóm, báo cáo chỉ giới thiệu
	sơ bộ về giao diện bản đồ chính của ứng dụng như trong Hình [fig:screen_map].

	
![Giao diện bản đồ chính của ứng dụng di động](Hinhve/screen-map.png)
*Hình: Giao diện bản đồ chính của ứng dụng di động*

	#### Giao diện hệ thống quản trị

	Hệ thống quản trị được thiết kế nhằm mục đích giám sát và phân tích dữ liệu
	toàn cảnh của hệ thống. Phân hệ này bao gồm nhiều tính năng, trong đó ba chức
	năng trọng tâm được minh họa bao gồm:

	**Giao diện tổng quan:** Giao diện chính chứa các bảng điều khiển thống kê
	(Hình [fig:screen_admin_panel]), hỗ trợ Ban quản lý nắm bắt nhanh các chỉ
	số hoạt động của hệ thống và các thông tin tổng hợp quan trọng.
	
![Giao diện tổng quan của hệ thống quản trị](Hinhve/screen-admin-panel.png)
*Hình: Giao diện tổng quan của hệ thống quản trị*

	**Quản lý và dự báo hành trình cá nhân:** Hình [fig:screen_admin_journey] minh họa giao diện theo dõi quỹ đạo di chuyển của từng người dùng cụ thể. Tại đây, quản trị viên có thể xem lại lịch sử các điểm đến của sinh viên, đồng thời hệ thống cung cấp tính năng dự đoán hành trình tiếp theo dựa trên mô hình không gian -- thời gian và thói quen cá nhân.
	
![Giao diện quản lý và dự báo hành trình cá nhân](Hinhve/screen-admin-journey.png)
*Hình: Giao diện quản lý và dự báo hành trình cá nhân*

	**Chế độ Quan sát trực tiếp \& Giả lập:** Hình [fig:screen_admin_simulation]
	hiển thị chế độ quan sát trực tiếp, cho phép quản trị viên theo dõi mật độ
	sinh viên thực tế trong khuôn viên trường thông qua lớp Heatmap. Bên cạnh đó,
	chế độ giả lập cho phép thiết lập các kịch bản sự kiện để dự báo dòng người
	phân bổ trên bản đồ.
	
![Giao diện quan sát trực tiếp và giả lập Heatmap](Hinhve/screen-admin-simulation.png)
*Hình: Giao diện quan sát trực tiếp và giả lập Heatmap*

	### Kết quả thu thập dữ liệu và đánh giá thuật toán

	Phần này tập trung trình bày kết quả thu thập dữ liệu thực tế và đánh giá hiệu năng của các thuật toán được áp dụng trong hệ thống, bao gồm thuật toán trích xuất điểm dừng và mô hình dự báo hành vi cá nhân.

	### Kết quả thu thập dữ liệu thực tế

	Dữ liệu được thu thập trong khoảng thời gian từ ngày 01/06/2026 đến ngày 26/06/2026 với sự tham gia của 23 người dùng là sinh viên Bách Khoa Hà Nội. Ứng dụng di động gửi tọa độ GPS về máy chủ với tần suất trung bình 15 giây/lần. Bảng~[table:data_collection_summary] tổng hợp các chỉ số chính của tập dữ liệu thu thập được.

	\begin{table}[H]
		\centering
		\caption{Tổng quan tập dữ liệu thu thập thực tế}
		
		\begin{tabular}{>{\raggedright\arraybackslash}p{9.5cm}>{\raggedleft\arraybackslash}p{4.5cm}}
			\toprule
			**Thông số** & **Giá trị** \\
			\midrule
			Số lượng người dùng thực tế tham gia & 23 \\[0.5em]
			Thời gian thu thập & 01/06 -- 26/06/2026 \\[0.5em]
			Tần suất gửi GPS & 15 giây/lần \\[0.5em]
			Tổng số điểm GPS thô người dùng gửi đến server & $\sim$1.500.000 \\[0.5em]
			Số điểm GPS ghi nhận trong khuôn viên trường & $\sim$132.000 \\[0.5em]
			Số lượng POI (tòa nhà/địa điểm) & 114 \\[0.5em]
			Tổng số điểm dừng (check-in sequence) & 1.157 \\[0.5em]
			Số check-in trung bình mỗi người dùng & $\sim$50 \\[0.5em]
			Thời gian dừng trung bình tại mỗi POI & $\sim$78 phút \\[0.5em]
			Sai số GPS lớn nhất & 3 -- 5 mét \\
			\bottomrule
		\end{tabular}
	\end{table}

	Có thể thấy, từ hơn 1,5 triệu điểm GPS thô, chỉ có khoảng 132.000 điểm nằm trong khuôn viên trường. Sau khi áp dụng thuật toán trích xuất điểm dừng, hệ thống đã rút gọn thành 1.157 chuỗi điểm dừng hợp lệ --- thể hiện khả năng lọc nhiễu và nén dữ liệu hiệu quả của pipeline xử lý. Thời gian dừng trung bình khoảng 78 phút tại mỗi tòa nhà tương ứng với thời lượng một buổi học hoặc một buổi tự học điển hình của sinh viên.

	### Đánh giá mô hình dự báo cá nhân

	Đối với mô hình dự báo cá nhân, tập dữ liệu được chia theo tỷ lệ 80/10/10 cho từng người dùng để huấn luyện và đánh giá, qua đó đảm bảo không xảy ra hiện tượng rò rỉ dữ liệu (data leakage).

	Để đánh giá độ chính xác của mô hình, báo cáo sử dụng hai thang đo phổ biến là tỷ lệ trúng đích (Hit Ratio -- HR@K) và thứ hạng trung bình nghịch đảo (Mean Reciprocal Rank -- MRR). Cụ thể:
	
		
-  **HR@K** đo lường tỷ lệ các dự đoán mà địa điểm thực tế được ghé thăm nằm trong danh sách $K$ địa điểm có xác suất dự báo cao nhất.
		
-  **MRR** đánh giá chất lượng xếp hạng của mô hình bằng cách lấy trung bình nghịch đảo của thứ hạng địa điểm đúng.
	

		Để có cơ sở so sánh và đánh giá hiệu quả của mô hình học máy đề xuất, chúng tôi thiết lập hai mô hình cơ sở (**Baseline**) phổ biến:
	
		
-  **Most Popular POI (Toàn cục):** Luôn đưa ra dự đoán địa điểm tiếp theo là địa điểm có tổng số lượt ghé thăm cao nhất trong tập huấn luyện của toàn bộ hệ thống (như thư viện, giảng đường lớn).
		
-  **Temporal-only ($P(temp)$):** Đưa ra dự đoán thuần túy dựa trên khung thời gian trong tuần (hour of week từ 0 đến 167), phản ánh tính chu kỳ theo thời khóa biểu học tập của sinh viên mà không kết hợp thói quen chuyển đổi vị trí và sở thích cá nhân.
	

	Kết quả đánh giá so sánh độ chính xác trên tập Test được tổng hợp trong Bảng~[table:prediction_eval].

	\begin{table}[H]
		\centering
		\caption{Kết quả đánh giá độ chính xác so với các mô hình Baseline}
		
		\small
		\renewcommand{\arraystretch}{1.3}
		\begin{tabularx}{\textwidth}{p{3.5cm} >{\centering\arraybackslash}X >{\centering\arraybackslash}X >{\centering\arraybackslash}X}
			\toprule
			**Chỉ số** & **Baseline (Most Popular)** & **Baseline (Temporal-only)** & **Mô hình đề xuất** \\
			\midrule
			HR@1 & 7,14\% & 16,67\% & 48,50\% \\[0.5em]
			HR@3 & 19,84\% & 37,30\% & 64,10\% \\[0.5em]
			HR@5 & 29,37\% & 52,38\% & 67,00\% \\[0.5em]
			MRR  & 0,197  & 0,315  & 0,579  \\
			\midrule
			\multicolumn{4}{l}{**Thông số cấu hình huấn luyện và kết quả tối ưu:**} \\[0.5em]
			Số mẫu Train / Val / Test & \multicolumn{3}{r}{917 / 114 / 103} \\[0.5em]
			Trọng số chuyển đổi vị trí ($\alpha$) & \multicolumn{3}{r}{0,094} \\[0.5em]
			Trọng số thời gian ($\beta$) & \multicolumn{3}{r}{0,520} \\[0.5em]
			Trọng số sở thích cá nhân ($\gamma$) & \multicolumn{3}{r}{0,386} \\
			\bottomrule
		\end{tabularx}
	\end{table}

	Từ bảng kết quả, có thể thấy mô hình đề xuất cho kết quả tốt hơn cả hai Baseline trên tất cả các chỉ số đo lường. Cụ thể, HR@1 đạt 48,50\% so với 7,14\% của Baseline phổ biến toàn cục và 16,67\% của Baseline chu kỳ thời gian. Chỉ số MRR đạt 0,579 (so với 0,197 và 0,315 của hai Baseline), tương đương với việc địa điểm thực tế xuất hiện trung bình ở vị trí thứ $\sim$1,7 trong danh sách gợi ý của mô hình, so với vị trí thứ $\sim$5,1 (Most Popular) và thứ $\sim$3,2 (Temporal-only). Kết quả này cho thấy việc kết hợp cả ba yếu tố cho kết quả tốt hơn so với chỉ sử dụng riêng lẻ từng thành phần. Lưu ý rằng các Baseline được chọn là mô hình toàn cục; một Baseline cá nhân hóa (Most Frequent POI per User) có thể cho kết quả cao hơn và sẽ là đối tượng so sánh trong các nghiên cứu tiếp theo.

	Đồng thời, kết quả tối ưu hóa hệ số trọng số chỉ ra rằng yếu tố thời gian chiếm trọng số lớn nhất ($\beta = 52,0\%$), phản ánh việc lịch học của sinh viên tuân theo thời khóa biểu cố định. Sở thích cá nhân đứng thứ hai ($\gamma = 38,6\%$), trong khi thói quen chuyển đổi vị trí tức thời đóng góp ít nhất ($\alpha = 9,4\%$) --- có thể do tập dữ liệu còn nhỏ chưa đủ để học được các quy luật chuyển tiếp ổn định. Mặc dù tập dữ liệu còn hạn chế về quy mô (103 mẫu test từ $\sim$23 người dùng), kết quả bước đầu khả quan, nhưng cần tập dữ liệu lớn hơn để xác nhận độ ổn định.

	### Cấu hình tham số mô phỏng cho bản đồ mật độ

	Nhằm phản ánh đặc điểm lưu thông trong khuôn viên trường, hệ thống cấu hình bộ tham số pha thời gian (Bảng [tab:campus_phase]) để điều chỉnh tỷ lệ sinh viên đang di chuyển (*Transit Ratio*) và hệ số mức độ nhộn nhịp (*Way Multiplier*) theo từng giai đoạn trong ngày. Các tham số này được lựa chọn nhằm phục vụ mục đích mô phỏng và trực quan hóa bản đồ mật độ dự báo, dựa trên đặc điểm hoạt động điển hình của khuôn viên trường. Do chưa được hiệu chỉnh bằng tập dữ liệu thực nghiệm quy mô lớn, các giá trị cấu hình hiện tại chỉ mang tính tham khảo và chưa được đánh giá định lượng về mức độ chính xác.

	\begin{table}[H]
		\centering
		\caption{Tham số cấu hình pha thời gian khuôn viên thực tế}
		
		\resizebox{\textwidth}{!}{
		\begin{tabular}{p{5.5cm}ccc}
			\toprule
			**Pha hoạt động** & **Khung giờ** & **Transit Ratio** & **Way Multiplier** \\
			\midrule
			ARRIVING              & 06:00--06:45 & 75\% & 1.5 \\
			IN\_CLASS             & Trong giờ học &  5\% & 0.3 \\
			SHIFT\_CHANGE         & Giữa ca học  & 65\% & 1.5 \\
			LUNCH\_RUSH           & 11:40--12:15 & 70\% & 1.2 \\
			LUNCH\_STAY\_ARRIVING & 12:15--12:30 & 40\% & 0.8 \\
			DEPARTING             & 17:25--18:00 & 80\% & 2.0 \\
			EVENING               & 18:00--21:00 & 20\% & 0.6 \\
			NIGHT                 & 21:00--06:00 & 10\% & 0.2 \\
			WEEKEND               & Ngày cuối tuần & 15\% & 0.5 \\
			\bottomrule
		\end{tabular}
		}
	\end{table}

	Bên cạnh đó, khi có sự kiện phát sinh, hệ thống tự động bổ sung lượng mật độ ảo tương đương với số lượng người dự kiến tham gia. Đồng thời, hệ số thu hút của khu vực cũng được nhân thêm với tỷ lệ $\left(1 + \frac{\text{số lượng}}{20}\right)$. Cơ chế khuyếch đại kép này giúp khu vực sự kiện luôn hiển thị rực sáng trên bản đồ nhiệt ngay cả khi thói quen di chuyển cá nhân chưa kịp thay đổi.

	## Kiểm thử

	Nhằm đảm bảo tính đúng đắn, độ ổn định và khả năng mở rộng của Hust Simulator, quá trình kiểm thử được thực hiện trên kiến trúc Microservice và cơ chế truyền phát dữ liệu thời gian thực.

	### Kiểm thử kiến trúc Microservice

	Do hệ thống được xây dựng theo kiến trúc Microservice, quá trình kiểm thử tập trung đánh giá khả năng phối hợp giữa các dịch vụ, tính nhất quán dữ liệu cũng như khả năng chịu lỗi khi xảy ra các sự cố trong quá trình vận hành. Các trường hợp kiểm thử được xây dựng nhằm mô phỏng cả điều kiện hoạt động bình thường lẫn các tình huống edge cases thường gặp trong hệ thống phân tán.

	Kết quả kiểm thử được tổng hợp trong Bảng [table:microservices_test].

	\begin{table}[H]
		\centering
		\caption{Các kịch bản kiểm thử đối với kiến trúc Microservice}
		
		\resizebox{\textwidth}{!}{
		\begin{tabular}{>{\raggedright\arraybackslash}p{0.8cm}>{\raggedright\arraybackslash}p{3.5cm}>{\raggedright\arraybackslash}p{4.7cm}>{\raggedright\arraybackslash}p{5.5cm}}
			\toprule **Mã** & **Kịch bản kiểm thử** & **Kết quả kỳ vọng** & **Kết quả thực tế / Số liệu đo lường** \\
			\midrule

			TC1 & **Cô lập lỗi**\newline Một service ngừng hoạt động. & Các service khác hoạt động bình thường, không xảy ra lỗi dây chuyền. & 100\% dịch vụ khác duy trì hoạt động; K8s tự khởi chạy lại Pod bị lỗi sau $\sim$12 giây. \\[1.5em]
			TC2 & **Xử lý Timeout**\newline Service phản hồi quá chậm. & Hệ thống ngắt kết nối đúng hạn, trả về lỗi phù hợp và không bị treo. & gRPC timeout ngắt chính xác ở 2000ms; trả về lỗi 504 Gateway Timeout ngay lập tức, giải phóng luồng xử lý. \\[1.5em]
			TC3 & **Phục hồi mạng**\newline Mất kết nối giữa các service. & Tự động kết nối lại khi mạng ổn định, không làm sai lệch dữ liệu. & Redis tự kết nối lại sau thời gian trễ lũy thừa; 0\% bản tin bị thất thoát trên Redis Stream. \\[1.5em]
			TC4 & **Đảm bảo bản tin**\newline Worker dừng khi đang xử lý message. & Message được đưa trở lại hàng đợi và tiếp tục được xử lý, không mất dữ liệu. & Sử dụng cơ chế manual ACK; 100\% bản tin chưa xác nhận được tự động đưa lại hàng đợi và xử lý lại bởi node khác. \\[1.5em]
			TC5 & **Tính lũy đẳng**\newline Gửi trùng lặp một yêu cầu. & Hệ thống nhận biết yêu cầu trùng lặp, tránh sinh dữ liệu dư thừa. & Lọc trùng qua Idempotency Key lưu trên Redis; 100\% request lặp lại nhận ngay kết quả cũ mà không tính toán lại. \\[1.5em]
			TC6 & **Xác thực dữ liệu**\newline Request chứa dữ liệu thiếu/sai định dạng. & Request bị từ chối ngay từ tầng API và thông tin lỗi được ghi nhận đầy đủ. & Kiểm tra qua Zod schema tại API Gateway trong $<3$ms; 100\% yêu cầu không hợp lệ bị chặn và trả về mã lỗi 400. \\[1.5em]
			TC7 & **Race condition**\newline Nhiều yêu cầu cùng cập nhật một tài nguyên. & Cơ chế đồng bộ hoạt động chính xác, đảm bảo dữ liệu nhất quán. & Áp dụng khóa optimistic lock kết hợp Redlock; 0\% lỗi xung đột hoặc ghi đè đè nén dữ liệu xảy ra. \\[1.5em]
			TC8 & **Thứ tự bản tin**\newline Các event đến không theo đúng thứ tự. & Trạng thái nghiệp vụ vẫn được duy trì chính xác và nhất quán. & Sử dụng sequence number nguồn; 100\% tin nhắn cũ hơn trạng thái hiện tại bị loại bỏ trực tiếp, bảo toàn tính đúng đắn. \\[1.5em]
			TC9 & **Zero-downtime Deployment**\newline Restart service khi đang nhận tải. & Lưu lượng được chuyển sang các node còn lại, người dùng không bị gián đoạn. & Rolling update phối hợp Readiness Probe; 0\% lỗi kết nối bị ghi nhận trong suốt quá trình thay đổi phiên bản. \\[1.5em]
			TC10 & **Xác thực trung tâm**\newline Token hết hạn hoặc không hợp lệ. & Request bị chặn tại API Gateway và trả về mã lỗi 401 Unauthorized. & Thời gian giải mã JWT đối xứng $<1,5$ms; 100\% request không có token hoặc hết hạn bị chặn tại API Gateway. \\
			\bottomrule
		\end{tabular}
		}
	\end{table}

	Kết quả kiểm thử cho thấy kiến trúc Microservice của Hust Simulator đáp ứng
	tốt yêu cầu về khả năng cô lập lỗi, duy trì tính nhất quán dữ liệu và đảm bảo tính
	sẵn sàng của hệ thống khi xảy ra các sự cố cục bộ. Các thành phần có thể hoạt
	động độc lập và phục hồi sau lỗi mà không làm ảnh hưởng đến hoạt động của toàn
	bộ hệ thống.
	### Kiểm thử cơ chế truyền phát thời gian thực AoI Pub/Sub

	Các kịch bản kiểm thử tải đối với luồng dữ liệu thời gian thực được thực hiện
	hoàn toàn trên Google Kubernetes Engine.
	Hệ thống sử dụng cụm GKE Autopilot, theo đó tài nguyên được cấp phát tự động dựa trên tổng mức cấu hình yêu cầu của các Pod thay vì cấu hình Node tĩnh. Tổng tài nguyên yêu cầu của toàn bộ các dịch vụ chỉ ở mức xấp xỉ **2 vCPU** và **4GB RAM**. Khi lượng kết nối đạt mức tải lớn nhất 10.000, tính năng Horizontal Pod Autoscaler đã tự động nhân bản các Pod, và Autopilot tự động dãn nở tài nguyên tương ứng để không bị nghẽn, đồng thời vẫn tối ưu tối đa chi phí hạ tầng.
	Các kịch bản tạo tải giả lập được thực thi bằng công cụ K6.

	Kết quả thu thập được với các kịch bản chịu tải khác nhau được trình bày trong Bảng
	[table:aoi_test_cases].

	\begin{table}[H]
		\centering
		\caption{Các kịch bản kiểm thử tải đối với cơ chế AoI Pub/Sub}
		
		\resizebox{\textwidth}{!}{
		\begin{tabular}{>{\raggedright\arraybackslash}p{3cm}>{\raggedright\arraybackslash}p{3.8cm}>{\raggedright\arraybackslash}p{7cm}}
			\toprule **Kịch bản kiểm thử** & **Thông số thiết lập**                     & **Đánh giá đo lường (Metrics)**                                                                                         \\
			\midrule Tải thông thường           & 1.000 user di chuyển qua WebSockets             & Hệ thống ổn định. Tải CPU của Broker $<15\%$, RAM $\sim 200\text{MB}$, độ trễ phát tán $<10\text{ms}$. \\
			Tải cao                             & 5.000 user di chuyển qua WebSockets             & CPU Broker tăng $45\%$, RAM $\sim 650\text{MB}$. Hệ thống phân tải tốt nhờ Zone Allocator, độ trễ $<25\text{ms}$. \\
			Tải lớn                             & 10.000 user di chuyển qua WebSockets            & K6 ghi nhận 54.998 phiên liên tục, 0 phiên rớt. Tải CPU Broker $\sim 60\%$, RAM $\sim 1.5\text{GB}$. Gateway xử lý mượt mà, độ trễ $<45\text{ms}$. \\
			Tải điểm nóng                       & Nhiều user đứng chung tại 1 điểm tụ tập         & Quá trình tính toán AoI không bị nghẽn, độ trễ vẫn đạt chuẩn thời gian thực $<15\text{ms}$.                          \\
			Di chuyển chéo khu vực              & Hàng loạt user băng qua ranh giới giữa 2 Broker & Không rớt bản tin. Cơ chế chuyển tiếp liên-Broker ghi nhận độ trễ chuyển tiếp nội bộ dưới $5\text{ms}$.         \\
			Gửi dữ liệu tần suất cao            & Client spam $>100$ bản tin tọa độ/s             & Gateway Rate Limiting tự động ngắt kết nối client spam, CPU của Broker không bị ảnh hưởng.                         \\
			\bottomrule
		\end{tabular}
		}
	\end{table}

	## Triển khai và giám sát hệ thống

	### Kiến trúc triển khai

	Hust Simulator được triển khai trên nền tảng đám mây Google Cloud Platform. Toàn bộ các dịch vụ được đóng gói dưới dạng Docker container và quản lý tập trung bằng Kubernetes thông qua dịch vụ Google Kubernetes Engine Autopilot. Cơ chế Autopilot tự động cung cấp hạ tầng máy chủ thay vì phải cấp phát Node tĩnh, qua đó tối ưu hóa tối đa chi phí vận hành.

	Kiến trúc mạng và Load Balancing được tổ chức theo nhiều lớp để đảm bảo hiệu năng cao. Ở lớp ngoài cùng, lưu lượng truy cập từ người dùng đi qua Google Cloud External Application Load Balancer. Bộ cân bằng tải này được GKE Ingress tự động khởi tạo, có khả năng phân tán lưu lượng lớn, tích hợp sẵn các biện pháp chống tấn công DDoS và hỗ trợ duy trì hàng chục nghìn kết nối đồng thời.

	Sau khi đi qua Load Balancer, các luồng yêu cầu được Ingress tiếp nhận và định tuyến vào các dịch vụ nội bộ tương ứng. Việc chia tách lưu lượng ngay từ cửa ngõ này giúp cô lập rủi ro và tăng cường khả năng chịu lỗi. Chi tiết các quy tắc định tuyến được trình bày trong Bảng [table:routing_rules].

	\begin{table}[H]
		\centering
		\caption{Phân chia các luồng định tuyến trên Ingress}
		
		\small
		\begin{tabular}{ll}
			\toprule **Tên miền (Domain)**         & **Mục đích định tuyến**          \\
			\midrule \texttt{admin.hustsimulator.id.vn} & Giao diện Quản trị viên               \\
			\texttt{observe.hustsimulator.id.vn}        & Bảng điều khiển giám sát hệ thống     \\
			\texttt{live.hustsimulator.id.vn}           & Máy chủ truyền tải luồng sự kiện thực \\
			*Mặc định (Default backend)*         & Phân giải và xử lý các yêu cầu API    \\
			\bottomrule
		\end{tabular}
	\end{table}

	Để bảo vệ an toàn truyền thông, toàn bộ kết nối từ người dùng đến Load Balancer đều được mã hóa bằng giao thức HTTPS. Chứng chỉ số SSL/TLS được cấp phát thông qua Google-managed SSL Certificates, giúp loại bỏ hoàn toàn rủi ro liên quan đến việc quên gia hạn chứng chỉ. Bên trong cụm Kubernetes, các Microservice giao tiếp với nhau qua mạng nội bộ (ClusterIP) kết hợp với hệ thống phân giải tên miền CoreDNS, đảm bảo các dữ liệu nhạy cảm không bị lộ ra Internet.

	Khả năng tự động mở rộng và cập nhật không gián đoạn là những đặc tính then chốt của kiến trúc. Hệ thống sử dụng Horizontal Pod Autoscaler để liên tục theo dõi mức sử dụng CPU/RAM, từ đó tự động tăng hoặc giảm số lượng replica của các service. Ngoài ra, trong các chu kỳ triển khai tích hợp liên tục (CI/CD), kiến trúc áp dụng chiến lược Rolling Update: khởi tạo Pod phiên bản mới, xác nhận trạng thái Readiness Probe thành công trước khi điều hướng luồng truy cập và loại bỏ dần Pod cũ, qua đó duy trì khả năng phục vụ liên tục.

	### Quản lý và phân bổ tài nguyên

	Hệ thống vận hành đồng thời khoảng 22 Pods bao gồm các service, cơ sở dữ liệu
	và công cụ giám sát. Để tránh tình trạng cạn kiệt tài nguyên cục bộ trên các Node,
	mỗi dịch vụ đều được thiết lập giới hạn mức tiêu thụ RAM và CPU tối thiểu và
	tối đa như trình bày trong Bảng [table:resource_allocation].

	\begin{table}[H]
		\centering
		\caption{Phân bổ tài nguyên cho các service trong K8s}
		
		\resizebox{\textwidth}{!}{
		\begin{tabular}{lllll}
			\toprule \multirow{2}{*}{**Tên dịch vụ**} & \multicolumn{2}{c}{**Requests (Tối thiểu)**} & \multicolumn{2}{c}{**Limits (Tối đa)**} \\
			\cmidrule(lr){2-3} \cmidrule(lr){4-5}          & **CPU**                                      & **Memory**                             & **CPU** & **Memory** \\
			\midrule API Gateway (\texttt{api-gateway})    & 50m                                               & 32Mi                                        & 250m         & 64Mi            \\
			Admin UI (\texttt{admin-ui})                   & 100m                                              & 128Mi                                       & 250m         & 256Mi           \\
			Auth Service (\texttt{auth-service})           & 100m                                              & 512Mi                                       & 500m         & 768Mi           \\
			Social Service (\texttt{social-service})       & 100m                                              & 512Mi                                       & 500m         & 768Mi           \\
			Context Service (\texttt{context-service})     & 100m                                              & 512Mi                                       & 500m         & 768Mi           \\
			Streaming Service (\texttt{streaming})         & 100m                                              & 512Mi                                       & 500m         & 768Mi           \\
			State Computation (\texttt{state-comp})        & 50m                                               & 64Mi                                        & 200m         & 128Mi           \\
			State Dissemination (\texttt{state-diss})      & 50m                                               & 64Mi                                        & 200m         & 128Mi           \\
			Prediction Service (\texttt{prediction})       & 500m                                              & 512Mi                                       & 1000m        & 1Gi             \\
			Interest Matcher (\texttt{matcher})            & 100m                                              & 32Mi                                        & 250m         & 64Mi            \\
			LiveKit Server (\texttt{livekit})              & 100m                                              & 128Mi                                       & 500m         & 256Mi           \\
			Message Broker (\texttt{rabbitmq})             & 100m                                              & 256Mi                                       & 500m         & 512Mi           \\
			In-memory DB (\texttt{redis})                  & 50m                                               & 64Mi                                        & 500m         & 64Mi            \\
			Tracing (\texttt{jaeger})                      & 100m                                              & 512Mi                                       & 250m         & 1Gi             \\
			Metrics (\texttt{prometheus})                  & 20m                                               & 128Mi                                       & 200m         & 256Mi           \\
			Dashboard (\texttt{grafana})                   & 10m                                               & 64Mi                                        & 100m         & 128Mi           \\
			Log Storage (\texttt{loki})                    & 10m                                               & 64Mi                                        & 100m         & 128Mi           \\
			\bottomrule
		\end{tabular}
		}
	\end{table}

	### Quan sát hệ thống

	Để vận hành ổn định, hệ thống thiết lập cơ chế giám sát xoay quanh ba trụ cột: Logs, Metrics và Traces. Toàn bộ dữ liệu được thu thập và tổng hợp thông qua các công cụ chuyên dụng, sau đó hiển thị trực quan (Hình~[fig:observability_dashboards]), giúp quản trị viên dễ dàng theo dõi và xử lý sự cố.

	
![Giao diện giám sát: Metrics (trái), Tracing trên Jaeger (giữa), và Logs (phải)](Hinhve/jvm-micrometer.png)
*Hình: Giao diện giám sát: Metrics (trái), Tracing trên Jaeger (giữa), và Logs (phải)*

	Bảng [table:observability_stack] tổng hợp chi tiết về bộ công cụ nguồn mở đã được áp dụng.

	\begin{table}[H]
		\centering
		\caption{Các công cụ trong hệ thống giám sát}
		
		\begin{tabular}{>{\raggedright\arraybackslash}p{2.5cm}>{\raggedright\arraybackslash}p{3.5cm}>{\raggedright\arraybackslash}p{7.5cm}}
			\toprule **Thành phần** & **Công cụ sử dụng** & **Mục đích / Vai trò**                                          \\
			\midrule Logs                & Promtail, Loki           & Thu thập và lưu trữ tập trung nhật ký hoạt động từ các container     \\
			Metrics                      & Prometheus               & Theo dõi, lưu trữ các chỉ số hiệu năng và mức tiêu thụ tài nguyên    \\
			Traces                       & OpenTelemetry, Jaeger    & Truy vết vòng đời yêu cầu qua các dịch vụ, giúp xác định điểm nghẽn  \\
			Dashboard                    & Grafana                  & Trực quan hóa toàn bộ dữ liệu giám sát trên một giao diện thống nhất \\
			\bottomrule
		\end{tabular}
	\end{table}

	Việc áp dụng bộ công cụ giám sát này đảm bảo khả năng theo dõi liên tục trạng thái hoạt động và hỗ trợ đắc lực công tác bảo trì hệ thống.


<!-- END OF 5_Trien_khai_danh_gia.tex -->
---


<!-- START OF 6_Giai_phap_dong_gop.tex (Chương 6: Các giải pháp và đóng góp nổi bật) -->



Chương này đi sâu vào phân tích các giải pháp kỹ thuật cốt lõi và những đóng góp mới trong kiến trúc của Hust Simulator. Mỗi đóng góp được cấu trúc chi tiết từ việc nhận diện nút thắt cổ chai, đề xuất giải pháp thiết kế, cho đến việc đánh giá hiệu năng thực tế.

## Mở rộng khả năng đồng bộ thời gian thực cho môi trường Digital Twin quy mô lớn

Kỹ thuật **Spatial Sharding** ở Chương 3 chỉ giải quyết bài toán ở mức độ vĩ mô. Bên trong mỗi phân vùng, việc sử dụng cơ chế **Full Broadcast** truyền thống (phát trạng thái của mỗi người dùng tới $N-1$ người dùng khác) sẽ tạo ra lượng thông điệp $O(N^2)$. Thực nghiệm thực tế cho thấy khi số lượng người dùng đồng thời vượt qua ngưỡng 1.000 thực thể di chuyển liên tục, cơ chế Full Broadcast gây bùng nổ băng thông mạng (vượt $10^6$ bản tin/giây), tràn hàng đợi I/O máy chủ và gây sập hàng loạt kết nối WebSocket thời gian thực. Đồng thời, client phải tiếp nhận và lọc bỏ lượng lớn thông tin thừa, làm hao pin và CPU của thiết bị di động.

![Cơ chế phân phối trạng thái Full Broadcast](Hinhve/FullBroadcast.png)
*Hình: Cơ chế phân phối trạng thái Full Broadcast*

Do phần lớn dữ liệu truyền đi giữa các khu vực địa lý xa nhau là không cần thiết, hệ thống bắt buộc phải tích hợp cơ chế lọc trạng thái theo vùng quan tâm (*Area of Interest -- AOI*). 

![Area of Interest (AOI)](Hinhve/aoi.png)
*Hình: Area of Interest (AOI)*

AOI định nghĩa một vùng không gian bao quanh người dùng mà người dùng quan tâm và có khả năng quan sát. Bằng cách chỉ đồng bộ trạng thái của các thực thể nằm trong vùng AOI này và lọc bỏ toàn bộ các cập nhật từ xa, hệ thống có thể tối ưu hóa tối đa lưu lượng mạng và chi phí xử lý.

Để hiện thực hóa cơ chế lọc này, các hệ thống thời gian thực thường áp dụng một số kỹ thuật quản lý không gian cơ bản như: lọc theo khoảng cách Euclid trực tiếp (Distance AOI) hoặc phân hoạch động dựa trên mật độ (Quadtree/Octree). Cụ thể, **Distance AOI** yêu cầu máy chủ liên tục tính toán khoảng cách hình học trực tiếp giữa mọi cặp đối tượng di động trên bản đồ và chỉ chuyển tiếp dữ liệu khi khoảng cách nhỏ hơn bán kính $R$ định trước. Phương pháp **Quadtree/Octree** lại phân chia không gian thành các nút cây phân cấp, tự động chia nhỏ vùng địa lý thành 4 vùng con (đối với Quadtree 2D) hoặc 8 vùng con (đối với Octree 3D) khi mật độ người dùng tại vùng đó vượt ngưỡng giới hạn. Để lựa chọn giải pháp tối ưu cho Hust Simulator, chúng tôi phân tích và so sánh các cơ chế này với Full Broadcast qua Bảng [table:aoi_comparison].

\begin{table}[H]
\centering
\caption{So sánh các cơ chế đồng bộ trạng thái trong môi trường Digital Twin}

\small
\renewcommand{\arraystretch}{1.3}
\newcolumntype{Y}{>{\raggedright\arraybackslash}X}
\begin{tabularx}{\textwidth}{p{2.3cm} Y Y Y Y}
\toprule
**Tiêu chí** &
**Full Broadcast** &
**Distance AOI** &
**Quadtree / Octree** &
**Multi-broker SPS** \\
\midrule

Phạm vi đồng bộ &
Toàn bộ thực thể &
Theo AOI &
Theo AOI &
Theo AOI \\

\midrule

Chi phí xử lý phía Server &
Thấp (không lọc) &
Rất cao (tính khoảng cách liên tục) &
Cao (duy trì cây động) &
Thấp (tra cứu ô lưới) \\

\midrule

Băng thông mạng &
Rất cao &
Thấp &
Thấp &
Thấp \\

\midrule

Độ trễ khi tải tăng &
Tăng nhanh &
Trung bình &
Trung bình &
Ổn định \\

\midrule

Khả năng mở rộng &
Thấp &
Trung bình &
Khá &
Rất cao \\

\midrule

Khả năng phân tán &
Không hỗ trợ &
Khó &
Hạn chế &
Cao \\

\midrule

Điểm nghẽn hệ thống &
Server trung tâm &
Server trung tâm &
Cấu trúc dữ liệu tập trung &
Phân tán qua nhiều Broker \\

\bottomrule
\end{tabularx}
\end{table}

Dựa trên kết quả so sánh ở Bảng [table:aoi_comparison], kiến trúc **Multi-broker Spatial Publish/Subscribe (Multi-broker SPS)**, lấy cảm hứng từ nghiên cứu của P.J. Smit và H.A. Engelbrecht [smit2024spatial], được lựa chọn làm giải pháp cốt lõi cho Hust Simulator. Kiến trúc này kết hợp phương pháp ánh xạ ô lưới cố định với mô hình phân mảnh không gian (Spatial Sharding) nhằm loại bỏ điểm nghẽn chịu lỗi và giới hạn băng thông card mạng.

Cụ thể, vùng không gian được chia thành các ô lưới có kích thước $10\text{m}\times10\text{m}$, trong đó mỗi ô được xem như một topic độc lập trong mô hình Publish/Subscribe. Một thực thể chỉ **Publish** trạng thái của mình vào ô lưới hiện tại, trong khi người dùng **Subscribe** các ô nằm trong phạm vi AOI tương ứng. Nhờ vậy, máy chủ không còn phải liên tục thực hiện các phép tính khoảng cách giữa mọi cặp thực thể như trong cách tiếp cận truyền thống. Thay vào đó, nhiệm vụ của máy chủ được đơn giản hóa thành việc chuyển tiếp bản tin tới các subscriber của từng ô lưới.

Kích thước lưới $10\text{m}\times10\text{m}$ được lựa chọn nhằm cân bằng giữa độ chính xác của AOI và chi phí bộ nhớ. Với AOI mặc định khoảng $30\text{m}\times30\text{m}$, mỗi người dùng chỉ cần theo dõi tập hợp $3\times3$ ô lưới lân cận thay vì toàn bộ các thực thể trong vùng. Nhờ đó, bài toán đối khớp liên tục trên tập dữ liệu lớn được chuyển thành các thao tác tra cứu cục bộ, giúp giảm đáng kể chi phí tính toán khi số lượng người dùng tăng cao.

Sau khi hoàn tất quá trình đối khớp AOI, hệ thống tiếp tục sử dụng thành phần **Dissemination Gateway** để tối ưu hóa quá trình phát tán dữ liệu tới thiết bị đầu cuối. Gateway đóng vai trò như một lớp trung gian giữa các Broker và client, chịu trách nhiệm đăng ký các luồng dữ liệu cần thiết, tổng hợp kết quả và phân phối lại tới thiết bị thông qua WebSocket. Nhờ đó, mỗi thiết bị di động chỉ cần duy trì một kết nối duy nhất và không phải tự thực hiện việc hợp nhất hay lọc dữ liệu từ nhiều Broker khác nhau, qua đó giảm tải tính toán cũng như mức tiêu thụ năng lượng.

Tuy nhiên, việc giảm chi phí đối khớp không đồng nghĩa với việc client luôn theo kịp tốc độ sinh dữ liệu của hệ thống. Nếu tốc độ tiêu thụ thấp hơn tốc độ cập nhật từ máy chủ, các bản tin sẽ liên tục tích tụ trong bộ đệm mạng, làm gia tăng độ trễ và có nguy cơ dẫn tới hiện tượng *cascading failure*. Để giải quyết vấn đề này, kiến trúc áp dụng cơ chế **Backpressure**. Khi phát hiện phía client không thể xử lý dữ liệu đủ nhanh, hệ thống sẽ chủ động loại bỏ các bản cập nhật trung gian đã lỗi thời và chỉ giữ lại trạng thái mới nhất. Nhờ đó, kích thước hàng đợi luôn được duy trì ở mức ổn định, ngăn chặn hiện tượng tràn bộ đệm và đảm bảo luồng dữ liệu thời gian thực không bị suy giảm khi quy mô hệ thống tiếp tục tăng lên.

Kết quả thực nghiệm cho thấy kiến trúc Multi-broker SPS giúp giảm đáng kể tải xử lý trên từng máy chủ, đồng thời duy trì độ trễ phát tán dưới $45\mathrm{ms}$ trong các kịch bản mô phỏng đồng thời 10.000 thực thể di động. Điều này cho thấy việc kết hợp Spatial Sharding và Multi-broker Spatial Publish/Subscribe đã tạo nên một kiến trúc có khả năng mở rộng tốt, đáp ứng yêu cầu đồng bộ thời gian thực của các hệ thống Digital Twin quy mô lớn.

## Thu thập và tiền xử lý dữ liệu hành vi di chuyển bằng thuật toán phát hiện điểm dừng

Để hiện thực hóa mô hình dự báo không gian -- thời gian trong môi trường thực tế, hệ thống yêu cầu một pipeline tiền xử lý có khả năng xử lý có hệ thống các đặc trưng phức tạp của dữ liệu thực tế ở cấp độ từng cá nhân. Ban đầu, thông tin thu thập được chỉ là các luồng tọa độ GPS thô từ thiết bị của mỗi sinh viên -- vốn chứa nhiều nhiễu định vị và không mang ý nghĩa ngữ cảnh. Mục tiêu của giai đoạn này là trích xuất các *điểm dừng có ý nghĩa* từ những chuỗi tọa độ rời rạc đó.

Bước đầu tiên là áp dụng thuật toán **Phát hiện điểm dừng** kết hợp ánh xạ POI và đối chiếu với điểm dừng lịch sử gần nhất để làm sạch dữ liệu. Quá trình xử lý chi tiết bao gồm việc gom cụm các điểm vị trí dao động trong một bán kính nhất định, bỏ qua các sai số nhiễu định vị, sau đó tiến hành đối khớp không gian với các POI và gộp/tách các lượt dừng kế tiếp của người dùng.
Chi tiết các bước thực thi của giải thuật được trình bày cụ thể trong mã giả dưới đây.

```text
[Giải thuật]
[H]
	{Thuật toán phát hiện điểm dừng}
	
	
	
	{Input}{Đầu vào}
	{Output}{Đầu ra}
	
	{
		Chuỗi GPS mới $P$; check-in cuối $s_{last}$; các ngưỡng $D_{max}, T_{min}, K_{max}, T_{merge}$ \\
		Trong đó: $D_{max} = 15{ m}$ (bán kính cụm), $T_{min} = 300{ s}$ (thời gian dừng tối thiểu) \\
		$K_{max} = 3$ (số điểm nhiễu tối đa), $T_{merge} = 2{ giờ}$ (khoảng cách gộp check-in)
	}
	{Bản ghi check-in mới được thêm hoặc cập nhật vào DB}
	
	$SP  []$, $i  1$\;
	{$i  n$}{
		$j  i + 1$, $spikes  0$, $Cluster  [p_i]$\;
		{$j  n$}{
			{${Haversine}(p_i, p_j) > D_{max}$}{
				$spikes  spikes + 1$\;
				{$spikes > K_{max}$}{
					$j  j - spikes$\;
					**break**\;
				}
			}
			{
				$spikes  0$\;
				$Cluster.{append}(p_j)$\;
			}
			$j  j + 1$\;
		}
		
		$duration  (Cluster[{last}].{time} - Cluster[1].{time}).{total\_seconds}()$\;
		{$duration  T_{min}$}{
			$lat_{avg}, lng_{avg}  {Centroid}(Cluster)$\;
			$SP.{append}((lat_{avg}, lng_{avg}, Cluster[1].{time}, Cluster[{last}].{time}))$\;
			$i  j$\;
		}
		{
			$i  i + 1$\;
		}
	}
	{mỗi $sp  SP$}{
		$poi  {SnapToPoi}(sp.{lat}, sp.{lng})$\;
		{$poi  {Null}$}{
			{$s_{last}  {Null}  s_{last}.poi = poi$}{
				$gap  |sp.{start\_time} - s_{last}.t|$\;
				{$gap  T_{merge}$}{
					$s_{last}.dur  s_{last}.dur + (sp.{end\_time} - s_{last}.t)$\;
					${UpdateDatabase}(s_{last})$\;
					**continue**\;
				}
			}
			$s_{new}  (poi, sp.{start\_time}, sp.{end\_time} - sp.{start\_time})$\;
			${InsertToDatabase}(s_{new})$\;
			$s_{last}  s_{new}$\;
		}
	}
```

Tiếp theo, hệ thống thực hiện phân tách hành trình và chuẩn bị dữ liệu check-in để huấn luyện mô hình dự báo. Nếu khoảng cách thời gian giữa hai điểm dừng liên tiếp vượt quá $2\text{ giờ}$, hệ thống sẽ phân tách quỹ đạo thành các hành trình con độc lập, nhằm tránh việc mô hình học các mối quan hệ nhân quả giả tạo giữa các hoạt động cách nhau quá xa. Cuối cùng, các hành trình sau khi phân tách nếu có ít hơn 3 điểm dừng sẽ bị loại bỏ khỏi tập dữ liệu huấn luyện, vì chuỗi quá ngắn không mang lại đủ thông tin ngữ cảnh cho mô hình Markov học các quy luật chuyển tiếp.
Nhờ các quy trình tiền xử lý chặt chẽ này, hệ thống xây dựng được các chuỗi hành trình hoàn chỉnh, đặc trưng và sạch nhiễu.

## Phân tích và dự báo hành vi di chuyển theo từng cá nhân

Sau khi dữ liệu hành trình được làm sạch và chuyển đổi thành chuỗi các điểm đến có ý nghĩa ở Mục [section:6.2], hệ thống tiến hành dự báo vị trí tiếp theo của từng sinh viên. Đóng góp thiết thực của giải pháp nằm ở việc thiết kế và hiện thực hóa mô hình lai **Context-Aware Recommender** kết hợp chuỗi Markov bậc một với các đặc trưng ngữ cảnh không gian -- thời gian thực tế, kèm cơ chế tự động học bộ trọng số tối ưu hóa cho mỗi người dùng.

Tại thời điểm dự báo $t$, hệ thống xác định địa điểm hiện tại $curr$ của người dùng và chuyển đổi thời gian mục tiêu thành chỉ số giờ-trong-tuần ($hw_{target} \in [0,167]$). Giả sử chuỗi lịch sử check-in của người dùng được mô tả bởi tập dữ liệu hành trình sạch $H = \{(loc_1, hw_1), (loc_2, hw_2), \dots, (loc_N, hw_N)\}$. Với mỗi địa điểm ứng viên $d$ trong khuôn viên trường, điểm số dự báo kết hợp $S(d)$ được định nghĩa như sau:

$$
    S(d) = \alpha \cdot P_{trans}(d \mid curr) + \beta \cdot P_{temp}(d \mid hw_{target}) + \gamma \cdot P_{pref}(d)
$$

Trong đó, ba đặc trưng ngữ cảnh thành phần được chúng tôi đề xuất tính toán cụ thể:

    
-  **Xác suất chuyển tiếp Markov ($P_{trans**$):} Thể hiện thói quen chuyển tiếp hành trình từ vị trí hiện tại $curr$ sang địa điểm tiếp theo $d$, ước lượng theo xác suất có điều kiện:
    
$$
        P_{trans}(d \mid curr) = \frac{\sum_{i=2}^{N} \mathbb{I}(loc_{i-1} = curr \land loc_i = d)}{\sum_{i=2}^{N} \mathbb{I}(loc_{i-1} = curr)}
    $$

    Với $\mathbb{I}(\cdot)$ là hàm chỉ thị, nhận giá trị $1$ nếu biểu thức đúng và $0$ nếu sai. Nếu người dùng chưa từng di chuyển từ vị trí $curr$ trong quá khứ, xác suất này được gán mặc định bằng $0$.

    
-  **Xác suất không gian -- thời gian dựa trên phân phối hạt nhân Gauss ($P_{temp**$):} Đo lường thói quen của sinh viên tại các thời điểm tương đồng trong tuần. Để tránh sự phân chia khung giờ cứng nhắc, chúng tôi áp dụng bộ lọc hạt nhân Gaussian (Gaussian KDE) với băng thông làm mượt $b = 2$ giờ:
    
$$
        P_{temp}(d \mid hw_{target}) = \frac{\sum_{i=1}^{N} \mathbb{I}(loc_i = d) \cdot K(hw_i, hw_{target})}{\sum_{j=1}^{N} K(hw_j, hw_{target})}
    $$

    Trong đó, hàm nhân Gauss $K(hw_i, hw_{target})$ được định nghĩa theo khoảng cách tuần hoàn thời gian trong tuần (nhằm nối liền ranh giới giữa cuối Chủ Nhật và đầu Thứ Hai):
    
$$
        K(hw_i, hw_{target}) = \exp \left( - \frac{\text{dist}^2(hw_i, hw_{target})}{2b^2} \right)
    $$

    
$$
        \text{dist}(hw_1, hw_2) = \min(|hw_1 - hw_2|, 168 - |hw_1 - hw_2|)
    $$

    
-  **Sở thích địa điểm nền ($P_{pref**$):} Thể hiện mức độ ưa thích tổng thể của người dùng đối với địa điểm $d$ trong lịch sử hoạt động:
    
$$
        P_{pref}(d) = \frac{\sum_{i=1}^{N} \mathbb{I}(loc_i = d)}{N}
    $$

Để cá nhân hóa mô hình cho từng sinh viên, bộ trọng số $(\alpha, \beta, \gamma)$ được học tự động thông qua quá trình huấn luyện định kỳ. Bằng cách sử dụng thuật toán tối ưu SLSQP trên phân hệ Prediction Service, hệ thống cực tiểu hóa hàm mất mát Categorical Cross-Entropy kết hợp cùng thành phần ràng buộc L2 Regularization hướng về bộ trọng số mặc định của hệ thống:

$$
    \min_{\alpha, \beta, \gamma} \left( -\frac{1}{|M|} \sum_{m \in M} \log(\hat{P}_{true\_dest}^m) + \lambda \cdot ||\mathbf{w} - \mathbf{w}_0||_2^2 \right)
$$

Với $M$ là tập mẫu huấn luyện, $\mathbf{w} = [\alpha, \beta, \gamma]^T$ và $\mathbf{w}_0 = [0.1, 0.5, 0.4]^T$ là bộ trọng số mặc định của hệ thống. Trong đó, $\beta$ đại diện cho thời khóa biểu có ảnh hưởng lớn nhất, tiếp theo là $\gamma$ biểu thị thói quen tổng thể và $\alpha$ biểu thị thói quen chuyển tiếp tức thời. Hệ số phạt $\lambda = 2.0$ dùng cho thành phần L2 Regularization để tránh quá khớp trên dữ liệu thưa thớt. Bài toán tối ưu hóa được giới hạn bởi các ràng buộc đẳng thức và bất đẳng thức:

$$
    \alpha + \beta + \gamma = 1 \quad \text{và} \quad \alpha, \beta, \gamma \ge 0
$$

Đối với các sinh viên mới chưa tích lũy đủ dữ liệu lịch sử di chuyển, mô hình sẽ tạm thời áp dụng bộ trọng số mặc định $\mathbf{w}_0$ nhằm vượt qua bài toán khởi đầu lạnh (cold-start). Nhờ cấu trúc kết hợp tuyến tính các đặc trưng thống kê gọn nhẹ thay vì sử dụng các mạng nơ-ron sâu phức tạp, mô hình dự báo đạt tốc độ phản hồi cực nhanh (chỉ ở mức mili-giây), đảm bảo khả năng xử lý thời gian thực cho hàng nghìn thiết bị hoạt động đồng thời trong Digital Twin.

## Kết hợp tri thức cá nhân để tổng hợp Bản đồ mật độ

Như đã trình bày ở Mục [section:6.3], hệ thống đã có khả năng dự báo ``sinh viên A sẽ đi đâu tiếp theo''. Nhưng với hàng nghìn người dùng cùng lúc, người quản trị không cần biết từng cá nhân đi đâu --- họ cần trả lời câu hỏi ở quy mô lớn hơn: *``đám đông sẽ tập trung ở đâu?''*. Đó chính là vai trò của **Bản đồ mật độ dự báo**.

Đóng góp của hệ thống nằm ở việc tích hợp mượt mà kết quả phân tích cá nhân với các quy luật không gian để xây dựng mô phỏng đám đông. Hệ thống vận hành hai lớp Heatmap song song và hoàn toàn độc lập:

    
-  **Live Heatmap:** Đếm trực tiếp số người dùng trong mỗi ô lưới từ dữ liệu GPS thực, cập nhật mỗi 5 giây, cho phép nắm bắt hiện trạng tức thời.
    
-  **Predictive Heatmap:** Tổng hợp hàng nghìn kết quả dự báo cá nhân, dàn trải lên hệ thống không gian thông qua hàm phân phối chuẩn và **Flocking Noise**, được cấu hình bằng bộ tham số cố định. Hệ thống có khả năng nội suy bản đồ mật độ cho một thời điểm trong tương lai.

Khả năng kết hợp giữa mô hình dự báo thời gian thực và mô phỏng giả lập --- nơi quản trị viên có thể tùy ý thay đổi pha thời gian, đóng/mở tòa nhà hay tạo sự kiện đột xuất --- đã giúp Hust Simulator vượt qua giới hạn của một công cụ hiển thị thông thường để trở thành một hệ thống Digital Twin toàn diện, hỗ trợ đắc lực cho bài toán phân tích rủi ro và lập kế hoạch điều phối đám đông.



<!-- END OF 6_Giai_phap_dong_gop.tex -->
---


<!-- START OF 7_Ket_luan.tex (Chương 7: Kết luận và hướng phát triển) -->



	## Kết luận

	Trong khuôn khổ đồ án, tôi đã xây dựng thành công bộ khung dành cho bản đồ số Digital Twin
	kết hợp mạng xã hội chuyên biệt dành cho sinh viên --- *Hust
	Simulator*. Hệ thống cho phép người dùng khám phá không gian khuôn viên trường,
	tương tác với các sự kiện trực tiếp, chia sẻ hành trình và kết nối với cộng đồng
	sinh viên. Sản phẩm được phát triển đa nền tảng với kiến trúc Microservices
	tiên tiến, sử dụng Java Spring Boot, NodeJS, Python kết hợp cùng các công nghệ hiện
	đại như gRPC, Redis Pub/Sub, WebRTC, giúp đảm bảo khả năng xử lý thời gian
	thực, độ trễ cực thấp và khả năng chịu tải tốt.

	So với các nền tảng mạng xã hội hoặc bản đồ số thông thường hiện nay, *Hust
	Simulator* có điểm khác biệt rõ rệt nhờ khả năng đồng bộ trạng thái vị trí theo
	thời gian thực. Việc kết hợp giữa thế giới ảo hóa khuôn viên trường và các
	tương tác xã hội thực tế giúp nâng cao trải nghiệm sinh viên, đồng thời tạo cơ hội
	để ban quản lý nhà trường quản lý các sự kiện quy mô lớn hiệu quả hơn.

	Trong suốt quá trình thực hiện Đồ án Tốt nghiệp, dưới sự hướng dẫn tận tình và
	chi tiết của *TS. Trịnh Thành Trung*, tôi đã học hỏi được rất nhiều về
	tối ưu hiệu năng kiến trúc phân tán, xử lý dữ liệu không gian -- thời gian, quy
	trình phát triển phần mềm, cũng như kỹ năng trình bày học thuật. Sự định hướng
	đúng đắn và động viên kịp thời của thầy đã giúp tôi vượt qua nhiều khó khăn để
	hoàn thành sản phẩm một cách trọn vẹn nhất.

	Bên cạnh những kết quả đã đạt được, hệ thống hiện vẫn tồn tại một số hạn chế kỹ thuật nhất định cần được thẳng thắn nhìn nhận:
	
		
-  **Quy mô dữ liệu thực nghiệm còn nhỏ:** Tập dữ liệu thử nghiệm thực tế chỉ gồm 23 người dùng và hơn 1.100 chuỗi điểm dừng trong vòng 1 tháng. Quy mô này chưa đủ lớn để phản ánh toàn diện các quy luật di chuyển phức tạp toàn trường, đồng thời hiện tượng dữ liệu thưa thớt cũng ảnh hưởng trực tiếp đến độ chính xác học các trọng số chuyển tiếp của mô hình dự báo.
		
-  **Đơn giản hóa trong mô hình dự báo:** Mô hình dự báo cá nhân hiện tại là sự kết hợp tuyến tính của 3 thành phần thông qua tối ưu hóa SLSQP. Hệ thống chưa tích hợp các kiến trúc học sâu phi tuyến phức tạp để khai thác mối quan hệ phi tuyến tính sâu hơn giữa các POI và ngữ cảnh thời gian.
	

	Đồ án này là kết quả của quá trình nghiên cứu và phát triển độc lập, với những đóng góp ở khía cạnh thiết kế kiến trúc hệ thống phân tán thời gian thực, kết hợp giữa bản đồ số Digital Twin và các tính năng tương tác mạng xã hội. Qua quá trình thực hiện, tôi đã tích lũy được nhiều bài học quý giá về quy trình xây dựng hệ thống microservices và tối ưu hiệu năng phân tán.

	## Hướng phát triển tiếp theo

	Để hoàn thiện và nâng cao hệ thống trong tương lai, hướng phát triển tiếp theo sẽ tập trung vào hai nhiệm vụ trọng tâm: nâng cấp các chức năng hiện có và mở rộng thêm các tiện ích thông minh. Đối với các chức năng hiện tại, hệ thống cần tiếp tục tối ưu hóa hiệu suất xử lý phía backend của thuật toán đối khớp không gian nhằm đáp ứng tốt khi số lượng người dùng đồng thời, sự kiện và dữ liệu di chuyển thực tế tăng lên quy mô lớn. Bên cạnh đó, giao diện quản trị 3D cần được nâng cấp để hỗ trợ các công cụ phân tích, thống kê dữ liệu không gian chuyên sâu, đồng thời tích hợp thêm module quản lý cấu trúc bản đồ trực tiếp trên giao diện web để tạo sự linh hoạt trong việc cập nhật thực tế.

	Song song với việc hoàn thiện, hệ thống định hướng mở rộng thêm các tính năng mới như mô phỏng luồng di chuyển của đám đông bằng trí tuệ nhân tạo để tự động lấp đầy không gian ảo, hoặc tích hợp công nghệ livestream thực tế ảo tăng cường kết hợp sự kiện thời gian thực. Hệ thống phân tích quỹ đạo di chuyển chuyên sâu sử dụng học sâu, kết hợp cùng các thuật toán gợi ý cá nhân hóa (Collaborative Filtering, Content-Based, Context-Aware) sẽ giúp tối ưu hóa khả năng kết nối bạn bè và gợi ý sự kiện phù hợp cho từng sinh viên. Cuối cùng, việc phát triển các module phân tích mật độ sinh viên trong các tòa nhà theo thời gian thực và nâng cấp hệ thống đánh giá sự kiện đa chiều (kèm hình ảnh, video) sẽ giúp hoàn thiện hệ sinh thái số.

	Tóm lại, đồ án đã hoàn thành mục tiêu xây dựng một nguyên mẫu thử nghiệm hệ thống bản đồ số thời gian thực kết hợp mạng xã hội sinh viên, xác thực tính khả thi của kiến trúc phân tán Spatial Sharding và mô hình dự báo hành vi. Việc khắc phục các hạn chế về quy mô dữ liệu và nâng cao độ ổn định của hệ thống dưới tải thực tế sẽ là những bước đi quyết định tiếp theo để đưa giải pháp ứng dụng vào thực tiễn.


<!-- END OF 7_Ket_luan.tex -->
---
