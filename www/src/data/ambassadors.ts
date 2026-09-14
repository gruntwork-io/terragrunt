/**
 * Terragrunt Ambassadors, lifted from the Webflow CMS collection.
 *
 * `image` is a filename under `src/assets/img/`, resolved at build time so a
 * typo fails the build instead of rendering a broken portrait.
 */

export interface Ambassador {
  name: string;
  role: string;
  /** Programme tenure badge, e.g. "Founding Ambassador". */
  since: string;
  image: string;
  bio: string;
  links: {
    github?: string;
    linkedin?: string;
    medium?: string;
    website?: string;
  };
}

export const ambassadors: Ambassador[] = [
  {
    "name": "Axel Mendoza",
    "role": "MLOps Engineer",
    "since": "Founding Ambassador",
    "image": "axel-mendoza.webp",
    "bio": "I'm a Senior MLOps Engineer with 6+ years of experience building production-ML systems. I write long-form articles on MLOps to help you build too!",
    "links": {
      "github": "https://github.com/ConsciousML",
      "linkedin": "https://www.linkedin.com/in/axelmdz/",
      "website": "https://www.axelmendoza.com/"
    }
  },
  {
    "name": "Juan Reyes",
    "role": "Lead Platform Engineer",
    "since": "Founding Ambassador",
    "image": "juan-reyes.jpeg",
    "bio": "Currently working in the Platform Engineering / DevOps space, I specialize in AWS, GCP, Infrastructure as Code (Terragrunt/OpenTofu/Terraform <3), and Kubernetes (I’m a proud Kubestronaut). I excel at solving complex infrastructure challenges by designing, building, and integrating scalable, robust, and cloud-native solutions. My passion lies in elevating infrastructure workflows by pushing the boundaries of automation, reusability, and best-practice architectures.",
    "links": {
      "github": "https://github.com/jfr992"
    }
  },
  {
    "name": "Steven Tseng",
    "role": "Staff DevOps Engineer",
    "since": "Founding Ambassador",
    "image": "steven-tseng.jpeg",
    "bio": "Steven Tseng is currently a leading DevOps engineer at Easygo. As part of day to day activities, Steven maintains a large number of cloud platforms & services to support a global fast-growing platform serving millions of customers, and generating thousands of transactions per second. Daily Terragrunt user, and a firm believer of IaC.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/tsengy/"
    }
  },
  {
    "name": "Dallas Slaughter",
    "role": "Founding Engineer",
    "since": "Founding Ambassador",
    "image": "dallas-slaughter.webp",
    "bio": "Dallas loves to build things almost as much as he loves to break them, and has plenty of experience at both to share. Having spent years as a DevOps engineer, someone got the bright idea to let him write code again and he's having a blast. Always happy to help, or listen, or just be the void that gets screamed into.",
    "links": {
      "github": "https://github.com/slaughtr",
      "linkedin": "https://www.linkedin.com/in/dallas-slaughter/"
    }
  },
  {
    "name": "Joshua Ward",
    "role": "DevOps Architect",
    "since": "Founding Ambassador",
    "image": "joshua-ward.webp",
    "bio": "I'm a DevOps enthusiast who loves building meaningful, high-impact solutions. I enjoy turning big-picture ideas into reality by choosing the right tools for the job. Terragrunt is often the first tool I reach for. It helps turn IaC chaos into organized, scalable, and maintainable infrastructure.",
    "links": {
      "github": "https://github.com/j2udev",
      "linkedin": "https://www.linkedin.com/in/joshua-ward-78997612a/"
    }
  },
  {
    "name": "Gabor Maghera",
    "role": "Sr. Manager, Platform Engineering",
    "since": "Founding Ambassador",
    "image": "gabor-maghera.jpeg",
    "bio": "I am a Senior Platform Engineering Manager with deep roots in hands-on engineering, passionate about architecting and scaling infrastructure-as-code solutions for the enterprise. I champion a DevOps culture where every developer can contribute infrastructure changes, using unobtrusive methods and minimally invasive feedback gates to ensure safety and security.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/gabormaghera/"
    }
  },
  {
    "name": "Amit Karni",
    "role": "DevOps Tech Lead",
    "since": "Ambassador since 2026",
    "image": "amit-karni.jpeg",
    "bio": "Shaping, running, and scaling production environments across AWS, Azure, and GCP from a DevOps Tech Lead, SRE, and platform engineering perspective. Big believer in DRY infrastructure. Terraform + Terragrunt is the backbone of how I build clean, scalable cloud environments. Here to help others get modular IaC right.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/amit-karni/",
      "medium": "https://medium.com/@amitkarni3293"
    }
  },
  {
    "name": "Sofiane Djerbi",
    "role": "Cloud Platform Architect",
    "since": "Ambassador since 2026",
    "image": "sofiane-djerbi.jpeg",
    "bio": "I'm a Cloud Platform Architect who focuses on building self-service cloud platforms using only what teams actually need. I favor simple, proven designs over over-engineering, and build platforms that remain reliable, operable, and cost-aware as systems and teams grow.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/sofianedjerbi/",
      "website": "https://cafe-cloud.com/"
    }
  },
  {
    "name": "Ceyda Duzgec",
    "role": "Solution Architect",
    "since": "Ambassador since 2026",
    "image": "ceyda-duzgec.jpeg",
    "bio": "Ceyda is a Solution Architect at Coca-Cola İçecek with a background in Python and microservices. She has extensive experience with AWS, Kubernetes, and using Terraform and Terragrunt for large production workloads. She also volunteers for various technology organizations, including Kubernetes Community Days Istanbul, and enjoys giving public talks on cloud-native technologies. In her free time, she likes playing tennis, snowboarding, and running.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/ceydaduzgec/"
    }
  },
  {
    "name": "Alexander Dovnar",
    "role": "Principle DevOps engineer",
    "since": "Ambassador since 2026",
    "image": "alexander-dovnar.jpeg",
    "bio": "Alexander Dovnar is an engineer focusing on Clouds & Infrastructure as Code, especially Terraform and Terragrunt. He helps teams build and manage cloud infrastructure in a clean, scalable, and predictable way. Alexander works a lot with structuring IaC, simplifying complex setups, and making infrastructure easier to maintain over time. In addition to engineering, he is a public speaker and technical educator, sharing practical DevOps and IaC experience through talks, workshops, podcasts & writing.",
    "links": {
      "github": "https://github.com/DovnarAlexander",
      "linkedin": "https://www.linkedin.com/in/dovnaralex/",
      "website": "https://www.youtube.com/@devopskitchentalks"
    }
  },
  {
    "name": "Srinivasulu Paranduru",
    "role": "AWS Solution & Devops Architect",
    "since": "Ambassador since 2026",
    "image": "srinivasulu-paranduru.jpeg",
    "bio": "Multi Cloud Solution Architect,Lead Devops engineer with 20+ years of IT experience | Microsoft Certified Trainer | AWS Community Builder & User Group Leader | GDG Organizer | Speaker & Blogger",
    "links": {
      "github": "https://github.com/srinivasuluparanduru",
      "linkedin": "https://www.linkedin.com/in/srinivasuluparanduru/"
    }
  },
  {
    "name": "Lorelei Rupp",
    "role": "Senior Principal DevOps Engineer",
    "since": "Ambassador since 2026",
    "image": "lorelei-rupp.jpeg",
    "bio": "I have been a Software Engineer for 20 years, 10 of those specifically as a DevOps engineer. My passion is to automate everything and I have really enjoyed working with Terraform and Terragrunt over the years. I hate all manual changes and if you ask me to do something manually, I will push back and automate it. I am currently responsible for the IAC of many environments at my current job and without Terragrunt I am not sure this would be possible. I am very excited to be a part of this community and help others on their IAC journey.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/loreleimccollum/"
    }
  },
  {
    "name": "Alex Torres",
    "role": "Senior Platform Engineer",
    "since": "Ambassador since 2026",
    "image": "alex-torres.jpeg",
    "bio": "Staff FullStack Engineer. Polyglot hands-on technical leader with strong expertise in Developer Experience, and IaC-at-Scale. I’ve spent years building platforms for massive companies (Warner Bros Discovery, TomTom), and I’ve learned that the best infrastructure is the kind developers barely notice, and the Platform Engineers love to maintain and scale.",
    "links": {
      "github": "https://github.com/Excoriate",
      "linkedin": "https://www.linkedin.com/in/alextorresruiz/"
    }
  },
  {
    "name": "Julius Omoleye",
    "role": "Senior DevOps Engineer",
    "since": "Ambassador since 2026",
    "image": "julius-omoleye.jpeg",
    "bio": "I am a Senior DevOps Engineer, AWS Network Architect, and Infrastructure Specialist with deep experience designing and operating scalable, secure cloud platforms using AWS, Kubernetes, and Infrastructure as Code.",
    "links": {
      "github": "https://github.com/geek0ps",
      "linkedin": "https://www.linkedin.com/in/julius-omoleye/",
      "medium": "https://devineer.medium.com/"
    }
  },
  {
    "name": "Yaroslav Naumenko",
    "role": "Senior Lead Cloud Architect",
    "since": "Ambassador since 2026",
    "image": "yaroslav-naumenko.jpeg",
    "bio": "I'm a Sr. Lead Cloud Architect at [Coupa Software](https://www.coupa.com/), where I build and manage enterprise GCP environments using Terragrunt, Terraform/Open Tofu, and GitOps across PCI-DSS, HIPAA, and FedRAMP regulated domains.",
    "links": {
      "github": "https://github.com/cloudon-one",
      "linkedin": "https://www.linkedin.com/in/ynaumenko/",
      "website": "https://blog.cloudon-one.com/"
    }
  },
  {
    "name": "Maneesh Karnati",
    "role": "Director of Software Engineering",
    "since": "Ambassador since 2026",
    "image": "maneesh-karnati.jpeg",
    "bio": "I’m Maneesh Karnati, currently leading platform engineering teams at MGM, where I focus on building scalable, well-governed infrastructure using infrastructure as code. Previously, I led cloud standardization, observability, and cost-optimization initiatives at AWS and Chewy, driving meaningful improvements in reliability, developer productivity, and cloud spend. I’m passionate about platform engineering, reducing operational toil, and building paved roads that let developers move fast while staying safe and I’m excited to contribute to the Terragrunt community.",
    "links": {
      "linkedin": "https://www.linkedin.com/in/maneeshkarnati/"
    }
  },
  {
    "name": "Adam Shero",
    "role": "Lead Platform Engineering",
    "since": "Ambassador since 2026",
    "image": "adam-shero.webp",
    "bio": "Adam is a Lead Platform Engineer in the media & entertainment industry. He has a wide technical background to draw from with nearly 30 years of experience at companies like DIRECTV, AT&T, and Dice. During his career in the Information Technology space, he has worked in many areas of tech ranging from desktop support, managing bare metal infrastructure, IT Service Management, DevOps, and Platform Engineering roles. His main area of expertise in the last six years involves AWS, GCP, Kubernetes, GitOps, Agentic Workflows, and leveraging Terragrunt at an enterprise level. He has a strong passion for developing repeatable patterns that scale, evangelize the importance of having consistent code, and codifying those standards for humans and agents. He also enjoys building and maintaining Terragrunt friendly public modules hosted in the Terraform Registry.",
    "links": {
      "github": "https://github.com/adamwshero",
      "linkedin": "https://www.linkedin.com/in/adamwshero/",
      "website": "https://cloudarmy.io/"
    }
  },
  {
    "name": "Andrew Babichev",
    "role": "Principal Infrastructure Engineer",
    "since": "Ambassador since 2026",
    "image": "andrew-babichev.webp",
    "bio": "I'm originally from Ukraine, right now live in UK. I have a wonderful family – wife, 2 daughters, cat, dog, and hamster. I started my career as a web developer (Ruby on Rails) and later switched to DevOps and cloud infrastructure engineering. I'm a big fan of Terraform/OpenTofu IaC approach and open source contributor to various providers and modules. I've been using Terragrunt since 2018.",
    "links": {
      "github": "https://github.com/Tensho",
      "linkedin": "https://www.linkedin.com/in/andrewbabichev/"
    }
  },
  {
    "name": "You",
    "role": "Terragrunt Champion",
    "since": "Lives and breathes all things Terragrunt.",
    "image": "you-placeholder-2.webp",
    "bio": "",
    "links": {}
  }
];
